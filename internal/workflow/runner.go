package workflow

import (
	"context"
	"fmt"
)

// Runner はステートマシンを驱動します。
// 後継 State が nil になるかエラーが発生するまで、現在の State の Execute を繰り返し呼び出します。
type Runner struct{}

// NewRunner は Runner を生成します。
func NewRunner() *Runner {
	return &Runner{}
}

// Run は initial からステートマシンを開始し、終了またはエラーまでステートを進めます。
func (r *Runner) Run(ctx context.Context, initial State, wfCtx *Context) error {
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
