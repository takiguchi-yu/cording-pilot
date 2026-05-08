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

// subtaskList は DecomposeTask の JSON レスポンス構造です。
type subtaskList struct {
	Subtasks []string `json:"subtasks"`
}

// CoderAgent は構造化されたコード生成を行うエージェントのインターフェースです。
type CoderAgent interface {
	// GenerateCode はタスク記述を元に構造化コード生成結果を返します。
	GenerateCode(ctx context.Context, task string) (CodeGenerationResult, error)
	// DecomposeTask は実装計画を独立して実装・テスト可能なサブタスクのリストに分割します。
	// LLM が分割できない場合や空リストを返した場合は、plan を単一要素のスライスとして返します。
	DecomposeTask(ctx context.Context, plan string) ([]string, error)
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

// DecomposeTask implements CoderAgent.
// JSON スキーマ {"subtasks": [...]} を用いて実装計画をサブタスクリストに分割します。
func (c *structuredCoder) DecomposeTask(ctx context.Context, plan string) ([]string, error) {
	prompt := fmt.Sprintf(
		"%s\n\n---\n\n以下の実装計画を、それぞれ独立して実装・テスト可能な小さなサブタスクのリストに分割してください。\n\n## 実装計画\n%s\n\n出力例: {\"subtasks\": [\"サブタスク1\", \"サブタスク2\"]}",
		c.systemPrompt,
		plan,
	)
	var result subtaskList
	if err := c.llm.GenerateStructured(ctx, prompt, &result); err != nil {
		return nil, fmt.Errorf("agent %s: decompose: %w", c.name, err)
	}
	if len(result.Subtasks) == 0 {
		// フォールバック: プラン全体を1つのサブタスクとして扱う。
		return []string{plan}, nil
	}
	return result.Subtasks, nil
}
