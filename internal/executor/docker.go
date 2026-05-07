package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// DefaultDockerImage は DockerExecutor が使用するデフォルトの Docker イメージです。
// golangci-lint, goimports, go コマンド一式が含まれます。
const DefaultDockerImage = "golangci/golangci-lint:latest"

const (
	defaultDockerTimeout = 10 * time.Minute
	worktreesDirName     = ".worktrees"
)

// DockerExecutor は git worktree で一時ブランチを切り出し、
// その内容を Docker コンテナにボリュームマウントして実行する Executor 実装です。
// Executor インターフェースを満たします。
type DockerExecutor struct {
	image   string
	timeout time.Duration
}

// NewDockerExecutor はデフォルトタイムアウト（10 分）で DockerExecutor を生成します。
// image に空文字を渡すと DefaultDockerImage を使用します。
func NewDockerExecutor(image string) *DockerExecutor {
	if image == "" {
		image = DefaultDockerImage
	}
	return &DockerExecutor{image: image, timeout: defaultDockerTimeout}
}

// NewDockerExecutorWithTimeout はカスタムイメージとタイムアウトで DockerExecutor を生成します。
// image に空文字を渡すと DefaultDockerImage を使用します。
func NewDockerExecutorWithTimeout(image string, timeout time.Duration) *DockerExecutor {
	if image == "" {
		image = DefaultDockerImage
	}
	return &DockerExecutor{image: image, timeout: timeout}
}

// Run は dir を git リポジトリルートとして一時ワークツリーを作成し、
// Docker コンテナ内でコマンドを実行します。
// 実行完了後（成功・失敗・パニック問わず）ワークツリーは defer により確実に削除されます。
// 非ゼロの終了コードは success=false で表し、err にはなりません。
func (e *DockerExecutor) Run(ctx context.Context, dir, cmd string, args ...string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
	branch := "worktree/" + taskID
	worktreeDir := filepath.Join(dir, worktreesDirName, taskID)

	if err := e.createWorktree(ctx, dir, worktreeDir, branch); err != nil {
		return "", false, fmt.Errorf("docker executor: create worktree: %w", err)
	}
	defer e.removeWorktree(dir, worktreeDir, branch)

	output, success, err := e.runInDocker(ctx, worktreeDir, cmd, args...)
	if err != nil {
		return output, false, fmt.Errorf("docker executor: run in container: %w", err)
	}
	return output, success, nil
}

// createWorktree は repoDir をリポジトリルートとして worktreeDir に
// branch ブランチの新しいワークツリーを作成します。
func (e *DockerExecutor) createWorktree(ctx context.Context, repoDir, worktreeDir, branch string) error {
	out, err := runGitCommand(ctx, repoDir, "worktree", "add", "-b", branch, worktreeDir, "HEAD")
	if err != nil {
		return fmt.Errorf("git worktree add: %w: %s", err, out)
	}
	return nil
}

// removeWorktree はワークツリーとブランチを確実に削除します。
// defer から呼ばれるため呼び出し元の ctx は受け取らず、
// 独立した 30 秒タイムアウトを設定してベストエフォートでクリーンアップします。
// エラーは標準エラー出力に記録します。
func (e *DockerExecutor) removeWorktree(repoDir, worktreeDir, branch string) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if out, err := runGitCommand(cleanupCtx, repoDir, "worktree", "remove", "--force", worktreeDir); err != nil {
		fmt.Fprintf(os.Stderr, "docker executor: cleanup: git worktree remove: %v: %s\n", err, out)
	}
	if out, err := runGitCommand(cleanupCtx, repoDir, "branch", "-D", branch); err != nil {
		fmt.Fprintf(os.Stderr, "docker executor: cleanup: git branch -D: %v: %s\n", err, out)
	}
}

// runInDocker は worktreeDir をコンテナの /workspace にマウントし、
// 指定されたコマンドを Docker コンテナ内で実行します。
// --rm フラグによりコンテナ終了時に自動削除されます。
func (e *DockerExecutor) runInDocker(ctx context.Context, worktreeDir, cmd string, args ...string) (string, bool, error) {
	dockerArgs := []string{
		"run", "--rm",
		"--volume", worktreeDir + ":/workspace",
		"--workdir", "/workspace",
		e.image,
		cmd,
	}
	dockerArgs = append(dockerArgs, args...)

	var buf bytes.Buffer
	c := exec.CommandContext(ctx, "docker", dockerArgs...)
	c.Stdout = &buf
	c.Stderr = &buf

	runErr := c.Run()
	output := buf.String()

	if runErr != nil {
		if ctx.Err() != nil {
			return output, false, fmt.Errorf("docker run timed out: %w", ctx.Err())
		}
		// 非ゼロ終了コード: コマンドの失敗であり、インフラエラーではない。
		return output, false, nil
	}
	return output, true, nil
}

// runGitCommand は repoDir 内で git コマンドを exec.CommandContext で実行し、
// 結合出力（stdout+stderr）とエラーを返します。
func runGitCommand(ctx context.Context, repoDir string, args ...string) (string, error) {
	var buf bytes.Buffer
	c := exec.CommandContext(ctx, "git", args...)
	c.Dir = repoDir
	c.Stdout = &buf
	c.Stderr = &buf
	err := c.Run()
	return buf.String(), err
}
