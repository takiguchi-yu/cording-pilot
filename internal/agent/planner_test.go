package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/takiguchi-yu/cording-pilot/internal/agent"
	"github.com/takiguchi-yu/cording-pilot/internal/config"
)

// plannerStubLLM は PlannerAgent のテスト用 LLM スタブです。
type plannerStubLLM struct {
	clarification agent.ClarificationRequest
	compiledIssue string
	generateErr   error
	structuredErr error
}

func (s *plannerStubLLM) Generate(_ context.Context, _ string) (string, error) {
	return s.compiledIssue, s.generateErr
}

func (s *plannerStubLLM) GenerateStructured(_ context.Context, _ string, target interface{}) error {
	if s.structuredErr != nil {
		return s.structuredErr
	}
	data, err := json.Marshal(s.clarification)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func TestFactory_NewPlannerAgent_GenerateClarification_質問あり(t *testing.T) {
	t.Parallel()

	want := agent.ClarificationRequest{
		IsClear: false,
		Questions: []agent.Question{
			{ID: "q1", Text: "スコープは？", Type: "text"},
		},
	}
	stub := &plannerStubLLM{clarification: want}
	f := agent.NewFactory(stub, config.DefaultGoConfig())
	pa := f.NewPlannerAgent()

	got, err := pa.GenerateClarification(context.Background(), "文字列を逆順にする関数")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IsClear {
		t.Error("IsClear should be false")
	}
	if len(got.Questions) != 1 || got.Questions[0].ID != "q1" {
		t.Errorf("unexpected questions: %+v", got.Questions)
	}
}

func TestFactory_NewPlannerAgent_GenerateClarification_要件明確(t *testing.T) {
	t.Parallel()

	want := agent.ClarificationRequest{IsClear: true}
	stub := &plannerStubLLM{clarification: want}
	f := agent.NewFactory(stub, config.DefaultGoConfig())
	pa := f.NewPlannerAgent()

	got, err := pa.GenerateClarification(context.Background(), "明確な要件")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsClear {
		t.Error("IsClear should be true")
	}
}

func TestFactory_NewPlannerAgent_GenerateClarification_LLMエラー時にエラーを返す(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("structured llm error")
	stub := &plannerStubLLM{structuredErr: wantErr}
	f := agent.NewFactory(stub, config.DefaultGoConfig())
	pa := f.NewPlannerAgent()

	_, err := pa.GenerateClarification(context.Background(), "要件")
	if !errors.Is(err, wantErr) {
		t.Errorf("expected wrapped error %v; got %v", wantErr, err)
	}
}

func TestFactory_NewPlannerAgent_CompileIssue_成功(t *testing.T) {
	t.Parallel()

	stub := &plannerStubLLM{compiledIssue: "## 実装計画\n..."}
	f := agent.NewFactory(stub, config.DefaultGoConfig())
	pa := f.NewPlannerAgent()

	got, err := pa.CompileIssue(context.Background(), "初期要件", map[string]string{"q1": "新規機能"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "## 実装計画\n..." {
		t.Errorf("unexpected compiled issue: %q", got)
	}
}

func TestFactory_NewPlannerAgent_CompileIssue_LLMエラー時にエラーを返す(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("generate error")
	stub := &plannerStubLLM{generateErr: wantErr}
	f := agent.NewFactory(stub, config.DefaultGoConfig())
	pa := f.NewPlannerAgent()

	_, err := pa.CompileIssue(context.Background(), "要件", map[string]string{})
	if !errors.Is(err, wantErr) {
		t.Errorf("expected wrapped error %v; got %v", wantErr, err)
	}
}

// TestFactory_NewPlannerAgent_AskはAgentインターフェースを満たすことを検証します。
func TestFactory_NewPlannerAgent_Askを呼び出す(t *testing.T) {
	t.Parallel()

	stub := &plannerStubLLM{compiledIssue: "plan output"}
	f := agent.NewFactory(stub, config.DefaultGoConfig())
	pa := f.NewPlannerAgent()

	// PlannerAgent は Agent を埋め込むため Ask も使用可能。
	resp, err := pa.Ask(context.Background(), "テストタスク")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "plan output" {
		t.Errorf("unexpected response: %q", resp)
	}
}
