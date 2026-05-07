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

const defaultNixTimeout = 15 * time.Minute

// NixExecutor は git worktree で一時ブランチを切り出し、
// リポジトリの flake.nix を使用して nix develop 環境内でコマンドを実行する Executor 実装です。
// Executor インターフェースを満たします。
type NixExecutor struct {
	timeout time.Duration
}

// NewNixExecutor はデフォルトタイムアウト（15 分）で NixExecutor を生成します。
// ホストマシンに nix コマンドが存在しない場合はエラーを返します。
func NewNixExecutor() (*NixExecutor, error) {
	if _, err := exec.LookPath("nix"); err != nil {
		return nil, fmt.Errorf("nix executor: Nix がインストールされていません。https://nixos.org/download.html からインストールしてください: %w", err)
	}
	return &NixExecutor{timeout: defaultNixTimeout}, nil
}

// NewNixExecutorWithTimeout はカスタムタイムアウトで NixExecutor を生成します。
// ホストマシンに nix コマンドが存在しない場合はエラーを返します。
func NewNixExecutorWithTimeout(timeout time.Duration) (*NixExecutor, error) {
	if _, err := exec.LookPath("nix"); err != nil {
		return nil, fmt.Errorf("nix executor: Nix がインストールされていません。https://nixos.org/download.html からインストールしてください: %w", err)
	}
	return &NixExecutor{timeout: timeout}, nil
}

// Run は dir を git リポジトリルートとして一時ワークツリーを作成し、
// flake.nix の devShell 内でコマンドを実行します。
// 実行完了後（成功・失敗・パニック問わず）ワークツリーは defer により確実に削除されます。
// 非ゼロの終了コードは success=false で表し、err にはなりません。
func (e *NixExecutor) Run(ctx context.Context, dir, cmd string, args ...string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
	branch := "worktree/" + taskID
	worktreeDir := filepath.Join(dir, worktreesDirName, taskID)

	if err := e.createWorktree(ctx, dir, worktreeDir, branch); err != nil {
		return "", false, fmt.Errorf("nix executor: create worktree: %w", err)
	}
	defer e.removeWorktree(dir, worktreeDir, branch)

	output, success, err := e.runInNix(ctx, worktreeDir, cmd, args...)
	if err != nil {
		return output, false, fmt.Errorf("nix executor: run in nix: %w", err)
	}
	return output, success, nil
}

// createWorktree は repoDir をリポジトリルートとして worktreeDir に
// branch ブランチの新しいワークツリーを作成します。
func (e *NixExecutor) createWorktree(ctx context.Context, repoDir, worktreeDir, branch string) error {
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
func (e *NixExecutor) removeWorktree(repoDir, worktreeDir, branch string) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if out, err := runGitCommand(cleanupCtx, repoDir, "worktree", "remove", "--force", worktreeDir); err != nil {
		fmt.Fprintf(os.Stderr, "nix executor: cleanup: git worktree remove: %v: %s\n", err, out)
	}
	if out, err := runGitCommand(cleanupCtx, repoDir, "branch", "-D", branch); err != nil {
		fmt.Fprintf(os.Stderr, "nix executor: cleanup: git branch -D: %v: %s\n", err, out)
	}
}

// runInNix は worktreeDir 内の flake.nix を参照して nix develop --command で
// 指定されたコマンドを Nix devShell 内で実行します。
func (e *NixExecutor) runInNix(ctx context.Context, worktreeDir, cmd string, args ...string) (string, bool, error) {
	nixArgs := []string{"develop", "--command", cmd}
	nixArgs = append(nixArgs, args...)

	fmt.Fprintf(os.Stderr, "nix executor: running %q in Nix devShell (dir: %s)\n", cmd, worktreeDir)

	var buf bytes.Buffer
	c := exec.CommandContext(ctx, "nix", nixArgs...)
	c.Dir = worktreeDir
	c.Stdout = &buf
	c.Stderr = &buf

	runErr := c.Run()
	output := buf.String()

	if runErr != nil {
		if ctx.Err() != nil {
			return output, false, fmt.Errorf("nix develop timed out after %s: %w", e.timeout, ctx.Err())
		}
		// 非ゼロ終了コード: コマンドの失敗であり、インフラエラーではない。
		return output, false, nil
	}
	return output, true, nil
}
