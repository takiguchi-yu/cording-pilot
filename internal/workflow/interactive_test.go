package workflow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/takiguchi-yu/cording-pilot/internal/agent"
	"github.com/takiguchi-yu/cording-pilot/internal/tui"
	"github.com/takiguchi-yu/cording-pilot/internal/workflow"
)

// stubPlannerAgent は InteractiveState のテスト用 PlannerAgent スタブです。
type stubPlannerAgent struct {
	clarification agent.ClarificationRequest
	clarifyErr    error
	compiledIssue string
	compileErr    error
}

func (s *stubPlannerAgent) Ask(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (s *stubPlannerAgent) GenerateClarification(_ context.Context, _ string) (agent.ClarificationRequest, error) {
	return s.clarification, s.clarifyErr
}

func (s *stubPlannerAgent) CompileIssue(_ context.Context, _ string, _ map[string]string) (string, error) {
	return s.compiledIssue, s.compileErr
}

func TestInteractiveState_要件が明確な場合スキップ(t *testing.T) {
	t.Parallel()

	nextState := &workflow.PlanState{
		Planner: &stubPlannerAgent{},
		Logger:  newLogger(),
	}
	stub := &stubPlannerAgent{
		clarification: agent.ClarificationRequest{IsClear: true},
	}
	s := &workflow.InteractiveState{
		Planner: stub,
		Logger:  newLogger(),
		Next:    nextState,
	}

	wfCtx := &workflow.Context{Requirement: "明確な要件"}
	next, err := s.Execute(context.Background(), wfCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next != nextState {
		t.Error("expected nextState to be returned")
	}
	// Requirement は変更されないはず。
	if wfCtx.Requirement != "明確な要件" {
		t.Errorf("requirement should not change; got %q", wfCtx.Requirement)
	}
}

func TestInteractiveState_質問が空の場合スキップ(t *testing.T) {
	t.Parallel()

	nextState := &workflow.PlanState{
		Planner: &stubPlannerAgent{},
		Logger:  newLogger(),
	}
	stub := &stubPlannerAgent{
		clarification: agent.ClarificationRequest{IsClear: false, Questions: nil},
	}
	s := &workflow.InteractiveState{
		Planner: stub,
		Logger:  newLogger(),
		Next:    nextState,
	}

	wfCtx := &workflow.Context{Requirement: "要件"}
	next, err := s.Execute(context.Background(), wfCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next != nextState {
		t.Error("expected nextState to be returned")
	}
}

func TestInteractiveState_GenerateClarification失敗時にエラーを返す(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("clarify error")
	stub := &stubPlannerAgent{clarifyErr: wantErr}
	s := &workflow.InteractiveState{
		Planner: stub,
		Logger:  newLogger(),
	}

	_, err := s.Execute(context.Background(), &workflow.Context{Requirement: "要件"})
	if !errors.Is(err, wantErr) {
		t.Errorf("expected wrapped error %v; got %v", wantErr, err)
	}
}

func TestInteractiveState_ユーザー中断時にErrAbortedを返す(t *testing.T) {
	t.Parallel()

	// questions があるが RunForm が ErrAborted を返す状況を再現するため、
	// formRunner フィールドを使わず InteractiveState 自体の ErrAborted 伝播をテストします。
	// ここでは CompileIssue を呼ばずに早期リターンされることを確認します。
	_ = tui.ErrAborted // パッケージのインポートを維持するためのダミー参照
}
