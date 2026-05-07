package agent

import (
	"context"
	"fmt"

	"github.com/takiguchi-yu/cording-pilot/internal/llm"
)

// FileUpdate は LLM が生成する単一ファイルの更新を表します。
type FileUpdate struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// CodeGenerationResult は LLM によるコード生成の出力を表します。
type CodeGenerationResult struct {
	Files []FileUpdate `json:"files"`
}

// CoderAgent は構造化されたコード生成を行うエージェントのインターフェースです。
type CoderAgent interface {
	// GenerateCode はタスク記述を元に構造化コード生成結果を返します。
	GenerateCode(ctx context.Context, task string) (CodeGenerationResult, error)
}

// structuredCoder は llm.Client.GenerateStructured を使用してコードを生成する CoderAgent 実装です。
type structuredCoder struct {
	name         string
	systemPrompt string
	llm          llm.Client
}

// GenerateCode implements CoderAgent.
func (c *structuredCoder) GenerateCode(ctx context.Context, task string) (CodeGenerationResult, error) {
	prompt := fmt.Sprintf("%s\n\n---\n\n%s", c.systemPrompt, task)
	var result CodeGenerationResult
	if err := c.llm.GenerateStructured(ctx, prompt, &result); err != nil {
		return CodeGenerationResult{}, fmt.Errorf("agent %s: %w", c.name, err)
	}
	return result, nil
}
