// Package agent は Agent インターフェースと、各エージェント（Planner/Coder/Reviewer）を
// 生成する Factory を定義します。
package agent

import "context"

// Agent はすべての専門 AI エージェントが実装する必要があるインターフェースです。
type Agent interface {
	// Ask はタスク記述をエージェントに送信し、テキスト形式の回答を返します。
	Ask(ctx context.Context, task string) (string, error)
}
