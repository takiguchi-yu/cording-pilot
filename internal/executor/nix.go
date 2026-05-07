package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

const defaultNixTimeout = 15 * time.Minute

// NixExecutor は指定されたディレクトリの flake.nix を使用して
// nix develop 環境内でコマンドを実行する Executor 実装です。
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

// Run は dir の flake.nix を利用して devShell 内でコマンドを実行します。
// 非ゼロの終了コードは success=false で表し、err にはなりません。
func (e *NixExecutor) Run(ctx context.Context, dir, cmd string, args ...string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	output, success, err := e.runInNix(ctx, dir, cmd, args...)
	if err != nil {
		return output, false, fmt.Errorf("nix executor: run in nix: %w", err)
	}
	return output, success, nil
}

// runInNix は workDir 内の flake.nix を参照して nix develop --command で
// 指定されたコマンドを Nix devShell 内で実行します。
func (e *NixExecutor) runInNix(ctx context.Context, workDir, cmd string, args ...string) (string, bool, error) {
	nixArgs := []string{"develop", "--command", cmd}
	nixArgs = append(nixArgs, args...)

	fmt.Fprintf(os.Stderr, "nix executor: running %q in Nix devShell (dir: %s)\n", cmd, workDir)

	var buf bytes.Buffer
	c := exec.CommandContext(ctx, "nix", nixArgs...)
	c.Dir = workDir
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
