package workflow

import (
	"context"
	"fmt"
	"os"
)

// Runner はステートマシンを驱動します。
// 後継 State が nil になるかエラーが発生するまで、現在の State の Execute を繰り返し呼び出します。
type Runner struct{}

// NewRunner は Runner を生成します。
func NewRunner() *Runner {
	return &Runner{}
}

// Run は initial からステートマシンを開始し、終了またはエラーまでステートを進めます。
// ワークフローの終了時（正常・エラー・シグナル問わず）に WorkDir を確実に削除します。
func (r *Runner) Run(ctx context.Context, initial State, wfCtx *Context) error {
	defer func() {
		if wfCtx.WorkDir != "" {
			if err := os.RemoveAll(wfCtx.WorkDir); err != nil {
				fmt.Fprintf(os.Stderr, "runner: cleanup: failed to remove work dir %s: %v\n", wfCtx.WorkDir, err)
			}
		}
	}()

	current := initial
	for current != nil {
		next, err := current.Execute(ctx, wfCtx)
		if err != nil {
			return fmt.Errorf("workflow runner: %w", err)
		}
		current = next
	}
	return nil
}
