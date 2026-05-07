package workflow

import (
	"context"
	"fmt"
	"os"

	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

// CompleteState は ④ 完了フェーズ—成功終了ステートです。
// 標準出力にサマリを表示し、NDJSON ログをフラッシュし、隔離作業ディレクトリを削除します。
type CompleteState struct {
	Logger *logger.Logger
}

// Execute は State を実装します。
// ワークフローの終了を示すため常に (nil, nil) を返します。
func (s *CompleteState) Execute(_ context.Context, wfCtx *Context) (State, error) {
	msg := fmt.Sprintf(
		"[COMPLETE] すべてのテストが %d 回の試行で通過しました。\n最後のテスト出力:\n%s",
		wfCtx.TryCount,
		wfCtx.LastTestOutput,
	)
	fmt.Println(msg)

	if err := s.Logger.Info("complete", msg); err != nil {
		return nil, err
	}

	// Best-effort cleanup of the isolated work directory.
	if wfCtx.WorkDir != "" {
		if err := os.RemoveAll(wfCtx.WorkDir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to remove work dir %s: %v\n", wfCtx.WorkDir, err)
		}
	}

	return nil, nil
}
