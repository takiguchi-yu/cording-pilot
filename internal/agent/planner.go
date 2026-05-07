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
	CompileIssue(ctx context.Context, requirement string, answers map[string]string, templateContent string) (string, error)
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

[CLARIFY] 以下の要件を分析し、実装品質を高めるために確認すべき事項を JSON で返してください。
要件の有無に関わらず、3〜5 件の確認質問（言語・エラーハンドリング・境界条件・テスト方針など）を必ず生成してください。
要件が全く提供されていない場合のみ is_clear を false に設定し、requirements が提供されている場合は is_clear は常に false としてください。

要件:
%s`, p.systemPrompt, requirement)

	var result ClarificationRequest
	// 注意: LLM が is_clear=true のまま questions を空で返すと TUI がスキップされるため、
	// IsClear は参照せず len(Questions) のみを判定基準とする（interactive.go 側の仕様）。
	if err := p.llm.GenerateStructured(ctx, prompt, &result); err != nil {
		return ClarificationRequest{}, fmt.Errorf("planner: generate clarification: %w", err)
	}
	return result, nil
}

// CompileIssue implements PlannerAgent.
func (p *plannerAgentImpl) CompileIssue(ctx context.Context, requirement string, answers map[string]string, templateContent string) (string, error) {
	var sb strings.Builder
	sb.WriteString("[COMPILE_ISSUE] 以下の要件とユーザーの回答を元に、実装計画（Markdown 形式の Issue）を生成してください。\n\n")
	sb.WriteString("この Issue は、実装前にコードベース全体を確認して方針を決めるための設計文書として利用されます。\n")
	sb.WriteString("そのため、既存実装との整合性を判断できる具体的な調査観点・影響範囲・方針決定基準を必ず含めてください。\n\n")
	sb.WriteString("## 出力形式の要件\n\n")
	sb.WriteString("**必ず以下の形式で出力してください：**\n\n")
	sb.WriteString("1. 先頭行に Issue タイトルを `# <タイトル>` 形式で記述すること。\n")
	sb.WriteString("   - タイトルはテンプレートのセクション名（例：概要、目的・背景）ではなく、このIssueで何をするかが一目でわかる具体的・簡潔な文言（50文字以内）にすること。\n")
	sb.WriteString("   - 例: `# ユーザー認証APIのJWT対応を実装する`\n\n")
	sb.WriteString("2. タイトル行の後に空行を挟み、以下の Issue テンプレートの構成・見出しに厳密に従って本文を記述すること。\n\n")
	sb.WriteString("3. **出力はMarkdownテキストをそのまま返すこと。コードブロック（\\`\\`\\`markdown や \\`\\`\\` など）で囲まないこと。**\n\n")
	sb.WriteString("4. 本文には、コードベース全体を調査して実装方針を決定するための情報として、少なくとも以下を含めること。\n")
	sb.WriteString("   - 影響範囲: 変更候補ディレクトリ/ファイル群、関連モジュール、依存関係\n")
	sb.WriteString("   - 調査方針: 既存コードをどの順序・観点で確認するか\n")
	sb.WriteString("   - 方針決定基準: どの条件で実装アプローチを選ぶか（例: 再利用優先、互換性維持、責務分離）\n")
	sb.WriteString("   - 検証観点: 追加/修正すべきテスト観点と受け入れ条件\n\n")
	sb.WriteString("## Issue テンプレート\n\n")
	if strings.TrimSpace(templateContent) == "" {
		sb.WriteString("(テンプレート未指定。一般的な見出し構成で Markdown の Issue を作成してください)\n\n")
	} else {
		sb.WriteString(templateContent)
		sb.WriteString("\n\n")
	}
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
	return stripOuterCodeFence(resp), nil
}

// stripOuterCodeFence は LLM が誤って ```markdown や ``` で全体を囲んだ場合にフェンスを除去します。
func stripOuterCodeFence(s string) string {
	trimmed := strings.TrimSpace(s)
	// ``` または ```markdown で始まる場合のみ除去する
	for _, prefix := range []string{"```markdown", "```"} {
		if strings.HasPrefix(trimmed, prefix) {
			rest := strings.TrimPrefix(trimmed, prefix)
			// 先頭の言語指定行（改行まで）を除去する
			if idx := strings.Index(rest, "\n"); idx >= 0 {
				rest = rest[idx+1:]
			}
			// 末尾の ``` を除去する
			if strings.HasSuffix(strings.TrimSpace(rest), "```") {
				rest = rest[:strings.LastIndex(rest, "```")]
			}
			return strings.TrimSpace(rest)
		}
	}
	return s
}

// NewPlannerAgent は要件の不足を分析して対話的に要件を確定する計画エージェントを生成します。
func (f *Factory) NewPlannerAgent() PlannerAgent {
	return &plannerAgentImpl{
		name:         "Planner",
		systemPrompt: f.cfg.Agents.Planner,
		llm:          f.llm,
	}
}
