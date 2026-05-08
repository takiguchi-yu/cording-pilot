package agent

import (
	"context"
	"fmt"

	"github.com/takiguchi-yu/cording-pilot/internal/llm"
)

// defaultSupervisorSystemPrompt は SupervisorAgent のデフォルトシステムプロンプトです。
const defaultSupervisorSystemPrompt = `あなたはシニアGoエンジニアの監督者（Supervisor）です。
Fix Loopで迷走しているCoderの状況を客観的に分析し、根本的な方針転換の助言を提供してください。
具体的なコード変更案や、別のアプローチを提示することが求められます。
助言は日本語で、簡潔かつ具体的に記述してください。`

// SupervisorAgent は Fix Loop の迷走を検知した際に Coder へ方針転換のアドバイスを与えるエージェントです。
type SupervisorAgent interface {
	// Advise は現在のコード、試行内容、エラー出力を元に、
	// Coder が解決できない理由と方針転換の助言を返します。
	Advise(ctx context.Context, currentCode, attempts, errorOutput string) (string, error)
}

// structuredSupervisor は llm.Client を使用してアドバイスを生成する SupervisorAgent 実装です。
type structuredSupervisor struct {
	name         string
	systemPrompt string
	llm          llm.Client
}

// Advise implements SupervisorAgent.
func (s *structuredSupervisor) Advise(ctx context.Context, currentCode, attempts, errorOutput string) (string, error) {
	prompt := fmt.Sprintf(
		"%s\n\n---\n\n## 現在のコード\n%s\n\n## 直近の試行内容\n%s\n\n## 出続けているエラー\n%s\n\n上記を分析し、なぜCoderは解決できないのか、どのような方針転換が必要かを日本語で具体的に助言してください。",
		s.systemPrompt,
		currentCode,
		attempts,
		errorOutput,
	)
	resp, err := s.llm.Generate(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("agent %s: advise: %w", s.name, err)
	}
	return resp, nil
}
