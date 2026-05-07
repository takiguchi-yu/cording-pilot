package agent

import (
	"context"
	"fmt"

	"github.com/takiguchi-yu/cording-pilot/internal/llm"
)

// baseAgent は llm.Client を固定のシステムプロンプトと組み合わせる内部ヘルパーです。
// システムプロンプトはユーザーのタスクの先頭に付加されます。
type baseAgent struct {
	name         string
	systemPrompt string
	llm          llm.Client
}

func (a *baseAgent) Ask(ctx context.Context, task string) (string, error) {
	prompt := fmt.Sprintf("%s\n\n---\n\n%s", a.systemPrompt, task)
	resp, err := a.llm.Generate(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("agent %s: %w", a.name, err)
	}
	return resp, nil
}

// Factory は指定した llm.Client を使用して専門エージェントインスタンスを生成します。
// 具象型を非公開に保ち、呼び出し元は Agent インターフェースのみに依存できます。
type Factory struct {
	llm llm.Client
}

// NewFactory は指定した LLM クライアントに接続した Factory を生成します。
func NewFactory(client llm.Client) *Factory {
	return &Factory{llm: client}
}

// NewPlanner は要件を分析して構造化された実装計画を作成する計画エージェントを生成します。
func (f *Factory) NewPlanner() Agent {
	return &baseAgent{
		name: "Planner",
		systemPrompt: `あなたは優秀なソフトウェアアーキテクトです。
与えられた要件を分析し、実装計画（目的・仕様・影響範囲）を日本語のMarkdown形式で出力してください。`,
		llm: f.llm,
	}
}

// NewCoder は Go のテストコードとプロダクトコードの両方を生成する実装エージェントを生成します。
// 後方互換のために残しています。新規コードには NewCoderAgent を使用してください。
func (f *Factory) NewCoder() Agent {
	return &baseAgent{
		name: "Coder",
		systemPrompt: `あなたは熟練のGoエンジニアです。
テストコードまたはプロダクトコードの生成を求められます。
コードのみを ` + "```go ... ```" + ` の形式で出力し、余分な説明は加えないでください。`,
		llm: f.llm,
	}
}

// NewCoderAgent は構造化された JSON 形式でコードを生成する実装エージェントを生成します。
// LLM に指定された JSON スキーマ（CodeGenerationResult）に従った出力を求めます。
func (f *Factory) NewCoderAgent() CoderAgent {
	return &structuredCoder{
		name: "Coder",
		systemPrompt: `あなたは熟練のGoエンジニアです。
テストコードまたはプロダクトコードの生成を求められます。
余分な説明やMarkdownコードブロックは含めず、以下のJSONスキーマに厳密に従って出力してください。

{"files":[{"path":"ファイルのパス","content":"ファイルの内容"}]}

- path にはリポジトリルートからの相対パスを指定してください。
- content にはファイルの完全な内容を文字列として指定してください。`,
		llm: f.llm,
	}
}

// NewReviewer は実装が元の要件を満たしているか検証するコードレビューエージェントを生成します。
func (f *Factory) NewReviewer() Agent {
	return &baseAgent{
		name: "Reviewer",
		systemPrompt: `あなたは厳格なコードレビュアーです。
差分と要件を突き合わせてレビューし、結果を "Approve" または "Request Changes" のいずれかで冒頭に明示してください。
問題点がある場合は具体的な修正点を列挙してください。`,
		llm: f.llm,
	}
}
