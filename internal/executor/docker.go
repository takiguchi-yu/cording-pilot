package executor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// DefaultDockerImage は DockerExecutor が使用するデフォルトの Docker イメージです。
// golangci-lint, goimports, go コマンド一式が含まれます。
const DefaultDockerImage = "golangci/golangci-lint:latest"

const (
	defaultDockerTimeout = 10 * time.Minute
)

// DockerExecutor は指定されたディレクトリを Docker コンテナにボリュームマウントして
// コマンドを実行する Executor 実装です。
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

// Run は dir をコンテナ内の /workspace としてマウントし、Docker コンテナ内でコマンドを実行します。
// 非ゼロの終了コードは success=false で表し、err にはなりません。
func (e *DockerExecutor) Run(ctx context.Context, dir, cmd string, args ...string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	output, success, err := e.runInDocker(ctx, dir, cmd, args...)
	if err != nil {
		return output, false, fmt.Errorf("docker executor: run in container: %w", err)
	}
	return output, success, nil
}

// runInDocker は workDir をコンテナの /workspace にマウントし、
// 指定されたコマンドを Docker コンテナ内で実行します。
// --rm フラグによりコンテナ終了時に自動削除されます。
func (e *DockerExecutor) runInDocker(ctx context.Context, workDir, cmd string, args ...string) (string, bool, error) {
	dockerArgs := []string{
		"run", "--rm",
		"--volume", workDir + ":/workspace",
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
