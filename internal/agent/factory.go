package agent

import (
	"context"
	"fmt"

	"github.com/takiguchi-yu/cording-pilot/internal/config"
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
	cfg *config.Config
}

// NewFactory は指定した LLM クライアントと設定に接続した Factory を生成します。
func NewFactory(client llm.Client, cfg *config.Config) *Factory {
	return &Factory{llm: client, cfg: cfg}
}

// NewCoder は Go のテストコードとプロダクトコードの両方を生成する実装エージェントを生成します。
// 後方互換のために残しています。新規コードには NewCoderAgent を使用してください。
func (f *Factory) NewCoder() Agent {
	return &baseAgent{
		name:         "Coder",
		systemPrompt: f.cfg.Agents.Coder,
		llm:          f.llm,
	}
}

// NewCoderAgent は構造化された JSON 形式でコードを生成する実装エージェントを生成します。
// LLM に指定された JSON スキーマ（CodeGenerationResult）に従った出力を求めます。
func (f *Factory) NewCoderAgent() CoderAgent {
	return &structuredCoder{
		name:         "Coder",
		systemPrompt: f.cfg.Agents.Coder,
		llm:          f.llm,
	}
}

// NewReviewer は実装が元の要件を満たしているか検証するコードレビューエージェントを生成します。
func (f *Factory) NewReviewer() Agent {
	return &baseAgent{
		name:         "Reviewer",
		systemPrompt: f.cfg.Agents.Reviewer,
		llm:          f.llm,
	}
}

// NewSupervisorAgent は Fix Loop 迷走時に Coder へ方針転換のアドバイスを行う Supervisor エージェントを生成します。
// cfg.Agents.Supervisor が設定されていない場合はデフォルトのシステムプロンプトを使用します。
func (f *Factory) NewSupervisorAgent() SupervisorAgent {
	prompt := f.cfg.Agents.Supervisor
	if prompt == "" {
		prompt = defaultSupervisorSystemPrompt
	}
	return &structuredSupervisor{
		name:         "Supervisor",
		systemPrompt: prompt,
		llm:          f.llm,
	}
}
