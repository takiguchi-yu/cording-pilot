package agent

import (
	"context"
	"fmt"

	"github.com/takiguchi-yu/cording-pilot/internal/llm"
)

// FilePatch は LLM が生成する単一ファイルのパッチを表します。
// 新規ファイルの場合は Content フィールドを使用します。
// 既存ファイルの修正には Search と Replace フィールドを使用します。
type FilePatch struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"` // 新規ファイル: ファイル全体の内容
	Search  string `json:"search,omitempty"`  // 既存ファイル修正: 置換対象の文字列（完全一致）
	Replace string `json:"replace,omitempty"` // 既存ファイル修正: 置換後の文字列
}

// CodeGenerationResult は LLM によるコード生成の出力を表します。
type CodeGenerationResult struct {
	Files []FilePatch `json:"files"`
}

// subtaskList は DecomposeTask の JSON レスポンス構造です。
type subtaskList struct {
	Subtasks []string `json:"subtasks"`
}

const (
	// testGenInstruction はテストコードのみを生成させる指示プレフィックスです。
	testGenInstruction = "[INSTRUCTION] テストコード（*_test.go）のみを生成してください。プロダクトコードは生成しないでください。テストは最初に必ず失敗（Red）する、意味のあるテストを書いてください。"
	// implGenInstruction はプロダクトコードのみを生成させる指示プレフィックスです。
	implGenInstruction = "[INSTRUCTION] プロダクトコードのみを生成してください。テストコードは生成しないでください。"
)

// CoderAgent は構造化されたコード生成を行うエージェントのインターフェースです。
type CoderAgent interface {
	// GenerateTest はタスク記述を元にテストコード（*_test.go）のみを生成します。
	// 生成されるテストは最初に必ず失敗（Red）する意味のあるテストである必要があります。
	GenerateTest(ctx context.Context, task string) (CodeGenerationResult, error)
	// GenerateImpl はタスク記述を元にプロダクトコードのみを生成します。
	// テストコードは生成しません。
	GenerateImpl(ctx context.Context, task string) (CodeGenerationResult, error)
	// GenerateCode はタスク記述を元に構造化コード生成結果を返します。
	// 後方互換のために残しています。新規コードには GenerateTest / GenerateImpl を使用してください。
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

// GenerateTest implements CoderAgent.
func (c *structuredCoder) GenerateTest(ctx context.Context, task string) (CodeGenerationResult, error) {
	prompt := fmt.Sprintf("%s\n\n%s\n\n---\n\n%s", c.systemPrompt, testGenInstruction, task)
	var result CodeGenerationResult
	if err := c.llm.GenerateStructured(ctx, prompt, &result); err != nil {
		return CodeGenerationResult{}, fmt.Errorf("agent %s: generate test: %w", c.name, err)
	}
	return result, nil
}

// GenerateImpl implements CoderAgent.
func (c *structuredCoder) GenerateImpl(ctx context.Context, task string) (CodeGenerationResult, error) {
	prompt := fmt.Sprintf("%s\n\n%s\n\n---\n\n%s", c.systemPrompt, implGenInstruction, task)
	var result CodeGenerationResult
	if err := c.llm.GenerateStructured(ctx, prompt, &result); err != nil {
		return CodeGenerationResult{}, fmt.Errorf("agent %s: generate impl: %w", c.name, err)
	}
	return result, nil
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
