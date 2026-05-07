package executor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

const defaultTimeout = 60 * time.Second

// LocalExecutor はローカルマシン上でコマンドを実行します。
// Executor インターフェースを満たします。
type LocalExecutor struct {
	timeout time.Duration
}

// NewLocalExecutor はデフォルトの 60 秒タイムアウトで LocalExecutor を生成します。
func NewLocalExecutor() *LocalExecutor {
	return &LocalExecutor{timeout: defaultTimeout}
}

// NewLocalExecutorWithTimeout はカスタムタイムアウトで LocalExecutor を生成します。
func NewLocalExecutorWithTimeout(timeout time.Duration) *LocalExecutor {
	return &LocalExecutor{timeout: timeout}
}

// Run は dir 内で cmd を args 付きで実行します。
// 呼び出しはエグゼキューターのタイムアウトと ctx にすでに設定された期限両方で制限されます。
// 非ゼロの終了コードは success=false で表し、err にはなりません。
// インフラ障害（タイムアウト、バイナリ晲見つからないなど）は err として返します。
func (e *LocalExecutor) Run(ctx context.Context, dir, cmd string, args ...string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	c := exec.CommandContext(ctx, cmd, args...)
	c.Dir = dir

	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf

	runErr := c.Run()
	output := buf.String()

	if runErr != nil {
		if ctx.Err() != nil {
			return output, false, fmt.Errorf("command timed out after %s: %w", e.timeout, ctx.Err())
		}
		// Non-zero exit: command failed, not an executor error.
		return output, false, nil
	}

	return output, true, nil
}
