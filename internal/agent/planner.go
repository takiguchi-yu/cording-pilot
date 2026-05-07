package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/takiguchi-yu/cording-pilot/internal/llm"
)

// Question は Planner Agent がユーザーへの確認のために生成する単一の質問です。
type Question struct {
	// ID は質問を一意に識別するキーです。回答マップのキーとして使用されます。
	ID string `json:"id"`
	// Text はユーザーに表示する質問文です。
	Text string `json:"text"`
	// Type は質問の入力形式です。"text"（自由記述）, "confirm"（Yes/No）, "select"（選択肢）のいずれかです。
	Type string `json:"type"`
	// Options は Type が "select" の場合に提示する選択肢のリストです。
	Options []string `json:"options"`
}

// ClarificationRequest は Planner Agent が要件分析後に返す確認要求です。
type ClarificationRequest struct {
	// Questions はユーザーへの確認事項のリストです。
	Questions []Question `json:"questions"`
	// IsClear は要件が十分に明確で質問が不要な場合に true となります。
	IsClear bool `json:"is_clear"`
}

// PlannerAgent は要件の不足を分析して質問リストを生成し、最終的な実装計画を作成する能力を持つエージェントです。
type PlannerAgent interface {
	Agent
	// GenerateClarification は要件を分析し、ユーザーへの確認事項を構造化して返します。
	GenerateClarification(ctx context.Context, requirement string) (ClarificationRequest, error)
	// CompileIssue はユーザーの回答を元に最終的な実装計画（Markdown 形式の擬似 Issue）を生成します。
	CompileIssue(ctx context.Context, requirement string, answers map[string]string) (string, error)
}

// plannerAgentImpl は PlannerAgent の具象実装です。
type plannerAgentImpl struct {
	name         string
	systemPrompt string
	llm          llm.Client
}

// Ask implements Agent.
func (p *plannerAgentImpl) Ask(ctx context.Context, task string) (string, error) {
	prompt := fmt.Sprintf("%s\n\n---\n\n%s", p.systemPrompt, task)
	resp, err := p.llm.Generate(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("agent %s: %w", p.name, err)
	}
	return resp, nil
}

// GenerateClarification implements PlannerAgent.
func (p *plannerAgentImpl) GenerateClarification(ctx context.Context, requirement string) (ClarificationRequest, error) {
	prompt := fmt.Sprintf(`%s

---

[CLARIFY] 以下の要件を分析し、実装前に確認すべき事項を JSON で返してください。
要件が十分に明確な場合は is_clear を true に設定し、questions を空にしてください。

要件:
%s`, p.systemPrompt, requirement)

	var result ClarificationRequest
	if err := p.llm.GenerateStructured(ctx, prompt, &result); err != nil {
		return ClarificationRequest{}, fmt.Errorf("planner: generate clarification: %w", err)
	}
	return result, nil
}

// CompileIssue implements PlannerAgent.
func (p *plannerAgentImpl) CompileIssue(ctx context.Context, requirement string, answers map[string]string) (string, error) {
	var sb strings.Builder
	sb.WriteString("[COMPILE_ISSUE] 以下の要件とユーザーの回答を元に、実装計画（Markdown 形式の擬似 Issue）を生成してください。\n\n")
	sb.WriteString("## 初期要件\n\n")
	sb.WriteString(requirement)
	sb.WriteString("\n\n## ユーザーの回答\n\n")
	for id, answer := range answers {
		fmt.Fprintf(&sb, "- **%s**: %s\n", id, answer)
	}

	prompt := fmt.Sprintf("%s\n\n---\n\n%s", p.systemPrompt, sb.String())
	resp, err := p.llm.Generate(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("planner: compile issue: %w", err)
	}
	return resp, nil
}

// NewPlannerAgent は要件の不足を分析して対話的に要件を確定する計画エージェントを生成します。
func (f *Factory) NewPlannerAgent() PlannerAgent {
	return &plannerAgentImpl{
		name: "Planner",
		systemPrompt: `あなたは優秀なソフトウェアアーキテクトです。
与えられた要件を分析し、実装前に明確にすべき事項を洗い出します。
質問は具体的かつ簡潔にし、実装上の意思決定に直結するものに絞ってください。`,
		llm: f.llm,
	}
}
