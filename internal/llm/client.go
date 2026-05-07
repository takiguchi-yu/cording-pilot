// Package llm は大規模言語モデル（LLM）と連携するための Strategy インターフェースを定義します。
package llm

import "context"

// Client は LLM 通信の Strategy インターフェースです。
// 実装はゴルーチン安全である必要があります。
type Client interface {
	// Generate はプロンプトを LLM に送信し、生成されたテキストを返します。
	Generate(ctx context.Context, prompt string) (string, error)
}
