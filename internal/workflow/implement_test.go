package workflow_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/takiguchi-yu/cording-pilot/internal/workflow"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

// stubExecutor は最小限の executor.Executor スタブです。
type stubExecutor struct {
	// responses は (output, success) ペアのキューで順に消費されます。
	responses []execResponse
	callCount int
}

type execResponse struct {
	output  string
	success bool
	err     error
}

func (s *stubExecutor) Run(_ context.Context, _, _ string, _ ...string) (string, bool, error) {
	if s.callCount >= len(s.responses) {
		return "", false, nil
	}
	r := s.responses[s.callCount]
	s.callCount++
	return r.output, r.success, r.err
}

func TestImplementState_Execute_初回イテレーションで成功する(t *testing.T) {
	t.Parallel()

	// Coder returns test code first, then correct impl code.
	callCount := 0
	coder := &funcAgent{fn: func(_ context.Context, task string) (string, error) {
		callCount++
		if strings.Contains(task, "[TEST_GEN]") {
			return "```go\npackage task\nfunc TestDummy(t *testing.T){}\n```", nil
		}
		return "```go\npackage task\n```", nil
	}}

	exec := &stubExecutor{
		responses: []execResponse{
			{output: "FAIL", success: false}, // initial Red
			{output: "ok", success: true},    // first Fix Loop iteration → Green
		},
	}

	next := &stubState{}
	s := &workflow.ImplementState{
		Coder:  coder,
		Exec:   exec,
		Logger: logger.New(&strings.Builder{}),
		Next:   next,
	}

	wfCtx := &workflow.Context{PlanText: "some plan"}
	got, err := s.Execute(context.Background(), wfCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != next {
		t.Errorf("expected Next state; got %v", got)
	}
	if wfCtx.TryCount != 1 {
		t.Errorf("TryCount=%d; want 1", wfCtx.TryCount)
	}
}

func TestImplementState_Execute_Fixループ上限到達でエラーを返す(t *testing.T) {
	t.Parallel()

	coder := &funcAgent{fn: func(_ context.Context, task string) (string, error) {
		if strings.Contains(task, "[TEST_GEN]") {
			return "```go\npackage task\nfunc TestDummy(t *testing.T){}\n```", nil
		}
		return "```go\npackage task\n```", nil
	}}

	// All runs fail.
	exec := &stubExecutor{
		responses: []execResponse{
			{output: "FAIL", success: false},
			{output: "FAIL", success: false},
			{output: "FAIL", success: false},
			{output: "FAIL", success: false},
		},
	}

	s := &workflow.ImplementState{
		Coder:  coder,
		Exec:   exec,
		Logger: logger.New(&strings.Builder{}),
		Next:   &stubState{},
	}

	_, err := s.Execute(context.Background(), &workflow.Context{PlanText: "plan"})
	if err == nil {
		t.Fatal("expected error when Fix Loop is exhausted, got nil")
	}
	if !strings.Contains(err.Error(), "上限") {
		t.Errorf("error should mention limit exhaustion; got: %v", err)
	}
}

func TestImplementState_Execute_テストレスポンスにコードブロックがない場合エラーを返す(t *testing.T) {
	t.Parallel()

	// Coder returns plain text with no code fence.
	coder := &funcAgent{fn: func(_ context.Context, _ string) (string, error) {
		return "no code block here", nil
	}}

	s := &workflow.ImplementState{
		Coder:  coder,
		Exec:   &stubExecutor{},
		Logger: logger.New(&strings.Builder{}),
		Next:   &stubState{},
	}

	_, err := s.Execute(context.Background(), &workflow.Context{PlanText: "plan"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestImplementState_Execute_テスト生成エージェントエラー時にエラーを返す(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("test gen agent error")
	coder := &funcAgent{fn: func(_ context.Context, _ string) (string, error) {
		return "", wantErr
	}}

	s := &workflow.ImplementState{
		Coder:  coder,
		Exec:   &stubExecutor{},
		Logger: logger.New(&strings.Builder{}),
		Next:   &stubState{},
	}

	_, err := s.Execute(context.Background(), &workflow.Context{PlanText: "plan"})
	if err == nil {
		t.Fatal("エラーを期待しましたが nil でした")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("wantErr をラップしているべきですが got: %v", err)
	}
}

func TestImplementState_Execute_初期テスト実行インフラエラー時にエラーを返す(t *testing.T) {
	t.Parallel()

	coder := &funcAgent{fn: func(_ context.Context, task string) (string, error) {
		if strings.Contains(task, "[TEST_GEN]") {
			return "```go\npackage task\nfunc TestDummy(t *testing.T){}\n```", nil
		}
		return "```go\npackage task\n```", nil
	}}

	infraErr := errors.New("exec infra error")
	exec := &stubExecutor{
		responses: []execResponse{
			{output: "", success: false, err: infraErr}, // 初期実行でインフラエラー
		},
	}

	s := &workflow.ImplementState{
		Coder:  coder,
		Exec:   exec,
		Logger: logger.New(&strings.Builder{}),
		Next:   &stubState{},
	}

	_, err := s.Execute(context.Background(), &workflow.Context{PlanText: "plan"})
	if err == nil {
		t.Fatal("エラーを期待しましたが nil でした")
	}
	if !errors.Is(err, infraErr) {
		t.Errorf("infraErr をラップしているべきですが got: %v", err)
	}
}

func TestImplementState_Execute_実装コード生成エージェントエラー時にエラーを返す(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("impl gen agent error")
	coder := &funcAgent{fn: func(_ context.Context, task string) (string, error) {
		if strings.Contains(task, "[TEST_GEN]") {
			return "```go\npackage task\nfunc TestDummy(t *testing.T){}\n```", nil
		}
		// 実装コード生成時にエージェントがエラーを返す。
		return "", wantErr
	}}

	exec := &stubExecutor{
		responses: []execResponse{
			{output: "FAIL", success: false}, // 初期実行 (Red)
		},
	}

	s := &workflow.ImplementState{
		Coder:  coder,
		Exec:   exec,
		Logger: logger.New(&strings.Builder{}),
		Next:   &stubState{},
	}

	_, err := s.Execute(context.Background(), &workflow.Context{PlanText: "plan"})
	if err == nil {
		t.Fatal("エラーを期待しましたが nil でした")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("wantErr をラップしているべきですが got: %v", err)
	}
}

func TestImplementState_Execute_Fixループでインフラエラー時にエラーを返す(t *testing.T) {
	t.Parallel()

	coder := &funcAgent{fn: func(_ context.Context, task string) (string, error) {
		if strings.Contains(task, "[TEST_GEN]") {
			return "```go\npackage task\nfunc TestDummy(t *testing.T){}\n```", nil
		}
		return "```go\npackage task\n```", nil
	}}

	infraErr := errors.New("fix loop infra error")
	exec := &stubExecutor{
		responses: []execResponse{
			{output: "FAIL", success: false},            // 初期実行 (Red)
			{output: "", success: false, err: infraErr}, // Fix Loop 実行でインフラエラー
		},
	}

	s := &workflow.ImplementState{
		Coder:  coder,
		Exec:   exec,
		Logger: logger.New(&strings.Builder{}),
		Next:   &stubState{},
	}

	_, err := s.Execute(context.Background(), &workflow.Context{PlanText: "plan"})
	if err == nil {
		t.Fatal("エラーを期待しましたが nil でした")
	}
	if !errors.Is(err, infraErr) {
		t.Errorf("infraErr をラップしているべきですが got: %v", err)
	}
}

// funcAgent は関数をバックエンドとする汎用 agent.Agentです。
type funcAgent struct {
	fn func(ctx context.Context, task string) (string, error)
}

func (a *funcAgent) Ask(ctx context.Context, task string) (string, error) {
	return a.fn(ctx, task)
}
