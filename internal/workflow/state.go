package workflow

import "context"

// State はすべてのワークフローフェーズが実装する必要があるインターフェースです。
// Execute はそのフェーズの処理を実行し、次に実行する State を返します。
// nil を返すことでワークフローの終了を示します。
type State interface {
	// Execute は wfCtx を共有の可変状態として使用してフェーズロジックを実行します。
	// 後継の State（終了なら nil）とエラーを返します。
	Execute(ctx context.Context, wfCtx *Context) (State, error)
}
