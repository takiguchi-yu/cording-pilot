package workflow_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/takiguchi-yu/cording-pilot/internal/agent"
	"github.com/takiguchi-yu/cording-pilot/internal/config"
	"github.com/takiguchi-yu/cording-pilot/internal/llm"
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

// singleStepConfig はテスト用のシンプルな 1 ステップパイプライン設定です。
// この config を Context に注入することで、テストの stubExecutor の呼び出し回数が
// デフォルトの 4 ステップ設定の影響を受けないようにします。
func singleStepConfig() *config.Config {
	return &config.Config{
		Version:     "1.0",
		Environment: config.Environment{Image: "golang:1.22"},
		Pipeline: []config.PipelineStep{
			{Name: "test", Command: "go test ./..."},
		},
	}
}

// testFiles は標準テストファイルセットを返します。
func testFiles() agent.CodeGenerationResult {
	return agent.CodeGenerationResult{
		Files: []agent.FilePatch{
			{Path: "task_test.go", Content: "package task\nfunc TestDummy(t *testing.T){}\n"},
		},
	}
}

// implFiles はシンプルな実装ファイルセットを返します。
func implFiles() agent.CodeGenerationResult {
	return agent.CodeGenerationResult{
		Files: []agent.FilePatch{
			{Path: "task.go", Content: "package task\n"},
		},
	}
}

func TestImplementState_Execute_初回イテレーションで成功する(t *testing.T) {
	t.Parallel()

	coder := &funcCoderAgent{fn: func(_ context.Context, task string) (agent.CodeGenerationResult, error) {
		if strings.Contains(task, "[TEST_GEN]") {
			return testFiles(), nil
		}
		return implFiles(), nil
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

	wfCtx := &workflow.Context{PlanText: "some plan", Config: singleStepConfig()}
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

	coder := &funcCoderAgent{fn: func(_ context.Context, task string) (agent.CodeGenerationResult, error) {
		if strings.Contains(task, "[TEST_GEN]") {
			return testFiles(), nil
		}
		return implFiles(), nil
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

	_, err := s.Execute(context.Background(), &workflow.Context{PlanText: "plan", Config: singleStepConfig()})
	if err == nil {
		t.Fatal("expected error when Fix Loop is exhausted, got nil")
	}
	if !strings.Contains(err.Error(), "上限") {
		t.Errorf("error should mention limit exhaustion; got: %v", err)
	}
}

func TestImplementState_Execute_テストファイルリストが空の場合エラーを返す(t *testing.T) {
	t.Parallel()

	// Coder returns an empty Files list.
	coder := &funcCoderAgent{fn: func(_ context.Context, _ string) (agent.CodeGenerationResult, error) {
		return agent.CodeGenerationResult{}, nil
	}}

	s := &workflow.ImplementState{
		Coder:  coder,
		Exec:   &stubExecutor{},
		Logger: logger.New(&strings.Builder{}),
		Next:   &stubState{},
	}

	_, err := s.Execute(context.Background(), &workflow.Context{PlanText: "plan", Config: singleStepConfig()})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestImplementState_Execute_テスト生成エージェントエラー時にエラーを返す(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("test gen agent error")
	coder := &funcCoderAgent{fn: func(_ context.Context, _ string) (agent.CodeGenerationResult, error) {
		return agent.CodeGenerationResult{}, wantErr
	}}

	s := &workflow.ImplementState{
		Coder:  coder,
		Exec:   &stubExecutor{},
		Logger: logger.New(&strings.Builder{}),
		Next:   &stubState{},
	}

	_, err := s.Execute(context.Background(), &workflow.Context{PlanText: "plan", Config: singleStepConfig()})
	if err == nil {
		t.Fatal("エラーを期待しましたが nil でした")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("wantErr をラップしているべきですが got: %v", err)
	}
}

func TestImplementState_Execute_初期テスト実行インフラエラー時にエラーを返す(t *testing.T) {
	t.Parallel()

	coder := &funcCoderAgent{fn: func(_ context.Context, task string) (agent.CodeGenerationResult, error) {
		if strings.Contains(task, "[TEST_GEN]") {
			return testFiles(), nil
		}
		return implFiles(), nil
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

	_, err := s.Execute(context.Background(), &workflow.Context{PlanText: "plan", Config: singleStepConfig()})
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
	coder := &funcCoderAgent{fn: func(_ context.Context, task string) (agent.CodeGenerationResult, error) {
		if strings.Contains(task, "[TEST_GEN]") {
			return testFiles(), nil
		}
		// 実装コード生成時にエージェントがエラーを返す（JSON パースエラーではない）。
		return agent.CodeGenerationResult{}, wantErr
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

	_, err := s.Execute(context.Background(), &workflow.Context{PlanText: "plan", Config: singleStepConfig()})
	if err == nil {
		t.Fatal("エラーを期待しましたが nil でした")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("wantErr をラップしているべきですが got: %v", err)
	}
}

func TestImplementState_Execute_Fixループでインフラエラー時にエラーを返す(t *testing.T) {
	t.Parallel()

	coder := &funcCoderAgent{fn: func(_ context.Context, task string) (agent.CodeGenerationResult, error) {
		if strings.Contains(task, "[TEST_GEN]") {
			return testFiles(), nil
		}
		return implFiles(), nil
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

	_, err := s.Execute(context.Background(), &workflow.Context{PlanText: "plan", Config: singleStepConfig()})
	if err == nil {
		t.Fatal("エラーを期待しましたが nil でした")
	}
	if !errors.Is(err, infraErr) {
		t.Errorf("infraErr をラップしているべきですが got: %v", err)
	}
}

func TestImplementState_Execute_パストラバーサルでエラーを返す(t *testing.T) {
	t.Parallel()

	// テスト生成で不正なパスを返す。
	coder := &funcCoderAgent{fn: func(_ context.Context, _ string) (agent.CodeGenerationResult, error) {
		return agent.CodeGenerationResult{
			Files: []agent.FilePatch{
				{Path: "../malicious.go", Content: "package main"},
			},
		}, nil
	}}

	s := &workflow.ImplementState{
		Coder:  coder,
		Exec:   &stubExecutor{},
		Logger: logger.New(&strings.Builder{}),
		Next:   &stubState{},
	}

	_, err := s.Execute(context.Background(), &workflow.Context{PlanText: "plan", Config: singleStepConfig()})
	if err == nil {
		t.Fatal("パストラバーサルエラーを期待しましたが nil でした")
	}
}

func TestImplementState_Execute_JSON解析エラーをFixLoopにフィードバックする(t *testing.T) {
	t.Parallel()

	// [TEST_GEN] → 正常, 1回目の実装 → JSON パースエラー, 2回目の実装 → 正常
	implCallCount := 0
	coder := &funcCoderAgent{fn: func(_ context.Context, task string) (agent.CodeGenerationResult, error) {
		if strings.Contains(task, "[TEST_GEN]") {
			return testFiles(), nil
		}
		implCallCount++
		if implCallCount == 1 {
			return agent.CodeGenerationResult{}, fmt.Errorf("agent Coder: %w", llm.ErrJSONParse)
		}
		return implFiles(), nil
	}}

	exec := &stubExecutor{
		responses: []execResponse{
			{output: "FAIL", success: false}, // 初期実行 (Red)
			{output: "ok", success: true},    // JSON エラー後の 2 回目 → Green
		},
	}

	next := &stubState{}
	s := &workflow.ImplementState{
		Coder:  coder,
		Exec:   exec,
		Logger: logger.New(&strings.Builder{}),
		Next:   next,
	}

	wfCtx := &workflow.Context{PlanText: "plan", Config: singleStepConfig()}
	got, err := s.Execute(context.Background(), wfCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != next {
		t.Errorf("expected Next state; got %v", got)
	}
	if implCallCount != 2 {
		t.Errorf("implCallCount=%d; want 2 (1回 JSON エラー + 1回 成功)", implCallCount)
	}
}

func TestImplementState_Execute_同一条件では実装生成キャッシュを再利用する(t *testing.T) {
	t.Parallel()

	implCallCount := 0
	coder := &funcCoderAgent{fn: func(_ context.Context, task string) (agent.CodeGenerationResult, error) {
		if strings.Contains(task, "[TEST_GEN]") {
			return testFiles(), nil
		}
		implCallCount++
		return implFiles(), nil
	}}

	// 初期実行 + 3回のFix Loopすべてで同じFAILを返す。
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

	_, err := s.Execute(context.Background(), &workflow.Context{PlanText: "plan", Config: singleStepConfig()})
	if err == nil {
		t.Fatal("expected Fix Loop exhaustion error")
	}
	if implCallCount != 1 {
		t.Errorf("implCallCount=%d; want 1 due to cache reuse", implCallCount)
	}
}

func TestImplementState_Execute_大きな失敗出力は切り詰めて実装生成へ渡す(t *testing.T) {
	t.Parallel()

	largeFailureOutput := strings.Repeat("failure-line\n", 1200)
	implPromptSeen := ""
	coder := &funcCoderAgent{fn: func(_ context.Context, task string) (agent.CodeGenerationResult, error) {
		if strings.Contains(task, "[TEST_GEN]") {
			return testFiles(), nil
		}
		implPromptSeen = task
		if strings.Contains(task, largeFailureOutput) {
			return agent.CodeGenerationResult{}, errors.New("large failure output should have been truncated")
		}
		return implFiles(), nil
	}}

	exec := &stubExecutor{
		responses: []execResponse{
			{output: largeFailureOutput, success: false},
			{output: "ok", success: true},
		},
	}

	next := &stubState{}
	s := &workflow.ImplementState{
		Coder:  coder,
		Exec:   exec,
		Logger: logger.New(&strings.Builder{}),
		Next:   next,
	}

	got, err := s.Execute(context.Background(), &workflow.Context{PlanText: "plan", Config: singleStepConfig()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != next {
		t.Errorf("expected Next state; got %v", got)
	}
	if !strings.Contains(implPromptSeen, "(snip") && !strings.Contains(implPromptSeen, "[truncated") {
		t.Errorf("impl prompt should include truncation marker; got %q", implPromptSeen)
	}
}

// funcCoderAgent は関数をバックエンドとする agent.CoderAgent スタブです。
type funcCoderAgent struct {
	fn          func(ctx context.Context, task string) (agent.CodeGenerationResult, error)
	decomposeFn func(ctx context.Context, plan string) ([]string, error)
}

// GenerateTest は内部的に [TEST_GEN] プレフィックスを付けて fn を呼び出します。
// これにより、fn の中で strings.Contains(task, "[TEST_GEN]") を使った既存ロジックが引き続き動作します。
func (a *funcCoderAgent) GenerateTest(ctx context.Context, task string) (agent.CodeGenerationResult, error) {
	return a.fn(ctx, "[TEST_GEN] "+task)
}

// GenerateImpl はプレフィックスなしで fn を呼び出します。
func (a *funcCoderAgent) GenerateImpl(ctx context.Context, task string) (agent.CodeGenerationResult, error) {
	return a.fn(ctx, task)
}

func (a *funcCoderAgent) GenerateCode(ctx context.Context, task string) (agent.CodeGenerationResult, error) {
	return a.fn(ctx, task)
}

// DecomposeTask は decomposeFn が設定されていればそれを呼び出し、
// 未設定の場合は plan を単一サブタスクとして返します（後方互換）。
func (a *funcCoderAgent) DecomposeTask(_ context.Context, plan string) ([]string, error) {
	if a.decomposeFn != nil {
		return a.decomposeFn(context.Background(), plan)
	}
	return []string{plan}, nil
}

// funcSupervisorAgent は関数をバックエンドとする agent.SupervisorAgent スタブです。
type funcSupervisorAgent struct {
	fn func(ctx context.Context, currentCode, attempts, errorOutput string) (string, error)
}

func (a *funcSupervisorAgent) Advise(ctx context.Context, currentCode, attempts, errorOutput string) (string, error) {
	return a.fn(ctx, currentCode, attempts, errorOutput)
}

func TestImplementState_Execute_PhaseA_テストが初回Greenの場合に再生成を要求する(t *testing.T) {
	t.Parallel()

	testGenCallCount := 0
	coder := &funcCoderAgent{fn: func(_ context.Context, task string) (agent.CodeGenerationResult, error) {
		if strings.Contains(task, "[TEST_GEN]") {
			testGenCallCount++
			return testFiles(), nil
		}
		return implFiles(), nil
	}}

	exec := &stubExecutor{
		responses: []execResponse{
			{output: "ok", success: true},    // Phase A 1回目: Green → 再試行
			{output: "FAIL", success: false}, // Phase A 2回目: Red ✓
			{output: "ok", success: true},    // Fix Loop → Green
		},
	}

	next := &stubState{}
	s := &workflow.ImplementState{
		Coder:  coder,
		Exec:   exec,
		Logger: logger.New(&strings.Builder{}),
		Next:   next,
	}

	wfCtx := &workflow.Context{PlanText: "plan", Config: singleStepConfig()}
	got, err := s.Execute(context.Background(), wfCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != next {
		t.Errorf("expected Next state; got %v", got)
	}
	if testGenCallCount != 2 {
		t.Errorf("testGenCallCount=%d; want 2 (1回 Green 再試行 + 1回 Red)", testGenCallCount)
	}
}

func TestImplementState_Execute_PhaseA_全試行がGreenの場合エラーを返す(t *testing.T) {
	t.Parallel()

	coder := &funcCoderAgent{fn: func(_ context.Context, task string) (agent.CodeGenerationResult, error) {
		if strings.Contains(task, "[TEST_GEN]") {
			return testFiles(), nil
		}
		return implFiles(), nil
	}}

	// maxTestRedRetries 回すべて Green を返す。
	exec := &stubExecutor{
		responses: []execResponse{
			{output: "ok", success: true},
			{output: "ok", success: true},
			{output: "ok", success: true},
		},
	}

	s := &workflow.ImplementState{
		Coder:  coder,
		Exec:   exec,
		Logger: logger.New(&strings.Builder{}),
		Next:   &stubState{},
	}

	_, err := s.Execute(context.Background(), &workflow.Context{PlanText: "plan", Config: singleStepConfig()})
	if err == nil {
		t.Fatal("エラーを期待しましたが nil でした")
	}
	if !strings.Contains(err.Error(), "Red") {
		t.Errorf("エラーは Red に関する内容であるべき; got: %v", err)
	}
}

func TestImplementState_Execute_ループ検知時にSupervisorを呼び出す(t *testing.T) {
	t.Parallel()

	supervisorCallCount := 0
	coder := &funcCoderAgent{fn: func(_ context.Context, task string) (agent.CodeGenerationResult, error) {
		if strings.Contains(task, "[TEST_GEN]") {
			return testFiles(), nil
		}
		return implFiles(), nil
	}}

	supervisor := &funcSupervisorAgent{fn: func(_ context.Context, _, _, _ string) (string, error) {
		supervisorCallCount++
		return "別のアプローチを試してください", nil
	}}

	exec := &stubExecutor{
		responses: []execResponse{
			{output: "FAIL", success: false}, // Phase A: Red ✓
			{output: "ERR", success: false},  // Fix Loop iter 0: 失敗
			{output: "ERR", success: false},  // Fix Loop iter 1: 同一エラー → Supervisor 呼び出し
			{output: "ok", success: true},    // Fix Loop iter 2: Green (Supervisor 助言後)
		},
	}

	next := &stubState{}
	s := &workflow.ImplementState{
		Coder:      coder,
		Supervisor: supervisor,
		Exec:       exec,
		Logger:     logger.New(&strings.Builder{}),
		Next:       next,
	}

	wfCtx := &workflow.Context{PlanText: "plan", Config: singleStepConfig()}
	got, err := s.Execute(context.Background(), wfCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != next {
		t.Errorf("expected Next state; got %v", got)
	}
	if supervisorCallCount != 1 {
		t.Errorf("supervisorCallCount=%d; want 1", supervisorCallCount)
	}
}

func TestImplementState_Execute_Supervisorがnilの場合はループ検知しても継続する(t *testing.T) {
	t.Parallel()

	coder := &funcCoderAgent{fn: func(_ context.Context, task string) (agent.CodeGenerationResult, error) {
		if strings.Contains(task, "[TEST_GEN]") {
			return testFiles(), nil
		}
		return implFiles(), nil
	}}

	// Supervisor は nil。同一エラーが続いても Fix Loop は通常通り継続する。
	exec := &stubExecutor{
		responses: []execResponse{
			{output: "FAIL", success: false}, // Phase A: Red ✓
			{output: "ERR", success: false},  // Fix Loop iter 0
			{output: "ERR", success: false},  // Fix Loop iter 1 (ループ検知 → Supervisor nil → スキップ)
			{output: "ok", success: true},    // Fix Loop iter 2: Green
		},
	}

	next := &stubState{}
	s := &workflow.ImplementState{
		Coder:      coder,
		Supervisor: nil, // Supervisor なし
		Exec:       exec,
		Logger:     logger.New(&strings.Builder{}),
		Next:       next,
	}

	wfCtx := &workflow.Context{PlanText: "plan", Config: singleStepConfig()}
	got, err := s.Execute(context.Background(), wfCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != next {
		t.Errorf("expected Next state; got %v", got)
	}
}
