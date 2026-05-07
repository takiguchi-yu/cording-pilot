package workflow_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/takiguchi-yu/cording-pilot/internal/workflow"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

// ── Stubs ────────────────────────────────────────────────────────────────────

// stubAgent は最小限の agent.Agent スタブです。
type stubAgent struct {
	response string
	err      error
}

func (s *stubAgent) Ask(_ context.Context, _ string) (string, error) {
	return s.response, s.err
}

// stubState は実行を記録する終端 State スタブです。
type stubState struct {
	executed bool
}

func (s *stubState) Execute(_ context.Context, _ *workflow.Context) (workflow.State, error) {
	s.executed = true
	return nil, nil
}

// newLogger は出力を破棄するテスト用ロガーを生成します。
func newLogger() *logger.Logger {
	return logger.New(&strings.Builder{})
}

// ── PlanState ────────────────────────────────────────────────────────────────

func TestPlanState_Execute_成功時に次のStateへ遷移する(t *testing.T) {
	t.Parallel()
	next := &stubState{}
	s := &workflow.PlanState{
		Planner: &stubAgent{response: "## plan"},
		Logger:  newLogger(),
		Next:    next,
	}
	wfCtx := &workflow.Context{Requirement: "reverse a string"}

	got, err := s.Execute(context.Background(), wfCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != next {
		t.Errorf("next state should be the injected stubState")
	}
	if wfCtx.PlanText != "## plan" {
		t.Errorf("PlanText=%q; want %q", wfCtx.PlanText, "## plan")
	}
}

func TestPlanState_Execute_エージェントエラー時にエラーを返す(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("llm down")
	s := &workflow.PlanState{
		Planner: &stubAgent{err: wantErr},
		Logger:  newLogger(),
		Next:    &stubState{},
	}

	_, err := s.Execute(context.Background(), &workflow.Context{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap wantErr; got: %v", err)
	}
}

// ── ReviewState ───────────────────────────────────────────────────────────────

func TestReviewState_Execute_承認時にOnApproveへ遷移する(t *testing.T) {
	t.Parallel()
	approve := &stubState{}
	reject := &stubState{}

	s := &workflow.ReviewState{
		Reviewer:  &stubAgent{response: "Approve – LGTM"},
		Logger:    newLogger(),
		OnApprove: approve,
		OnReject:  reject,
	}
	wfCtx := &workflow.Context{
		Requirement:    "req",
		PlanText:       "plan",
		LastTestOutput: "ok",
	}

	got, err := s.Execute(context.Background(), wfCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != approve {
		t.Errorf("expected OnApprove state; got %v", got)
	}
}

func TestReviewState_Execute_修正要求時にOnRejectへ遷移する(t *testing.T) {
	t.Parallel()
	approve := &stubState{}
	reject := &stubState{}

	s := &workflow.ReviewState{
		Reviewer:  &stubAgent{response: "Request Changes: fix the loop"},
		Logger:    newLogger(),
		OnApprove: approve,
		OnReject:  reject,
	}
	wfCtx := &workflow.Context{
		Requirement:    "req",
		PlanText:       "plan",
		LastTestOutput: "FAIL",
		TryCount:       2,
	}

	got, err := s.Execute(context.Background(), wfCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != reject {
		t.Errorf("expected OnReject state; got %v", got)
	}
	// TryCount should be reset.
	if wfCtx.TryCount != 0 {
		t.Errorf("TryCount=%d; want 0 after rejection", wfCtx.TryCount)
	}
	// Feedback should be appended to PlanText.
	if !strings.Contains(wfCtx.PlanText, "fix the loop") {
		t.Errorf("review feedback should be appended to PlanText; got: %q", wfCtx.PlanText)
	}
}

func TestReviewState_Execute_レビュアーエラー時にエラーを返す(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("reviewer unavailable")
	s := &workflow.ReviewState{
		Reviewer:  &stubAgent{err: wantErr},
		Logger:    newLogger(),
		OnApprove: &stubState{},
		OnReject:  &stubState{},
	}

	_, err := s.Execute(context.Background(), &workflow.Context{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap wantErr; got: %v", err)
	}
}

// ── CompleteState ─────────────────────────────────────────────────────────────

func TestCompleteState_Execute_nilを返しワークフローを終了する(t *testing.T) {
	t.Parallel()
	s := &workflow.CompleteState{Logger: newLogger()}
	wfCtx := &workflow.Context{TryCount: 2, LastTestOutput: "ok\n"}

	next, err := s.Execute(context.Background(), wfCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next != nil {
		t.Errorf("CompleteState should return nil next state; got %v", next)
	}
}

func TestCompleteState_Execute_WorkDirを削除する(t *testing.T) {
	t.Parallel()
	s := &workflow.CompleteState{Logger: newLogger()}
	dir := t.TempDir()
	wfCtx := &workflow.Context{WorkDir: dir, TryCount: 1, LastTestOutput: "ok"}

	_, err := s.Execute(context.Background(), wfCtx)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Errorf("WorkDir %q は削除されているはずですが存在します", dir)
	}
}

// ── Runner ────────────────────────────────────────────────────────────────────

func TestRunner_Run_単一Stateを実行する(t *testing.T) {
	t.Parallel()
	stub := &stubState{}
	r := workflow.NewRunner()
	err := r.Run(context.Background(), stub, &workflow.Context{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stub.executed {
		t.Error("stubState should have been executed")
	}
}

func TestRunner_Run_State連鎖を順に実行する(t *testing.T) {
	t.Parallel()
	// chain: a → b → nil
	b := &stubState{}
	a := &chainState{next: b}

	r := workflow.NewRunner()
	err := r.Run(context.Background(), a, &workflow.Context{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !a.executed {
		t.Error("state a should have been executed")
	}
	if !b.executed {
		t.Error("state b should have been executed")
	}
}

func TestRunner_Run_Stateエラー時にエラーを返す(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("state boom")
	bad := &errState{err: wantErr}

	r := workflow.NewRunner()
	err := r.Run(context.Background(), bad, &workflow.Context{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap wantErr; got: %v", err)
	}
}

// ── ヘルパー State 型 ──────────────────────────────────────────────────────────

type chainState struct {
	executed bool
	next     workflow.State
}

func (c *chainState) Execute(_ context.Context, _ *workflow.Context) (workflow.State, error) {
	c.executed = true
	return c.next, nil
}

type errState struct {
	err error
}

func (e *errState) Execute(_ context.Context, _ *workflow.Context) (workflow.State, error) {
	return nil, e.err
}
