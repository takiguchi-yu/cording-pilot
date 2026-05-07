package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/takiguchi-yu/cording-pilot/internal/agent"
	"github.com/takiguchi-yu/cording-pilot/internal/executor"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
	"github.com/takiguchi-yu/cording-pilot/pkg/markdown"
)

const (
	maxTryCount  = 3
	goModContent = "module task\n\ngo 1.21\n"
)

// ImplementState は ② 実装フェーズ（Fix Loop 含む）です。
// TDD サイクル（テスト生成→実行（Red）→実装生成→実行（Green or Red））を最大 maxTryCount 回繰り返します。
// 成功時は Next（ReviewState）へ遷移し、上限到達時はエラーを返します。
type ImplementState struct {
	Coder  agent.Agent
	Exec   executor.Executor
	Logger *logger.Logger
	// Next は全テスト通過時の後継ステート（通常は ReviewState）です。
	Next State
}

// Execute implements State.
func (s *ImplementState) Execute(ctx context.Context, wfCtx *Context) (State, error) {
	if err := s.Logger.Info("implement.start", "② 実装フェーズを開始します"); err != nil {
		return nil, err
	}

	// Set up the isolated working directory.
	workDir, err := os.MkdirTemp("", "cording-pilot-*")
	if err != nil {
		return nil, fmt.Errorf("implement: create work dir: %w", err)
	}
	wfCtx.WorkDir = workDir

	if err = os.WriteFile(filepath.Join(workDir, "go.mod"), []byte(goModContent), 0o600); err != nil {
		return nil, fmt.Errorf("implement: write go.mod: %w", err)
	}

	// Step 1: generate test code.
	testCode, err := s.generateTestCode(ctx, wfCtx)
	if err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(workDir, "task_test.go"), []byte(testCode), 0o600); err != nil {
		return nil, fmt.Errorf("implement: write test file: %w", err)
	}

	// Step 2: initial run (expected Red — no implementation yet).
	out, _, execErr := s.Exec.Run(ctx, workDir, "go", "test", "./...")
	if execErr != nil {
		return nil, fmt.Errorf("implement: initial test run: %w", execErr)
	}
	wfCtx.LastTestOutput = out
	if err = s.Logger.Info("implement.initial_test", fmt.Sprintf("初期テスト実行 (Red 想定)\n%s", out)); err != nil {
		return nil, err
	}

	// Step 3: Fix Loop.
	for wfCtx.TryCount < maxTryCount {
		wfCtx.TryCount++

		if err = s.Logger.Info(
			"implement.fix_loop",
			fmt.Sprintf("Fix Loop 試行 %d/%d", wfCtx.TryCount, maxTryCount),
		); err != nil {
			return nil, err
		}

		implCode, genErr := s.generateImplCode(ctx, wfCtx)
		if genErr != nil {
			return nil, genErr
		}
		if err = os.WriteFile(filepath.Join(workDir, "task.go"), []byte(implCode), 0o600); err != nil {
			return nil, fmt.Errorf("implement: write impl file: %w", err)
		}

		out, success, execErr := s.Exec.Run(ctx, workDir, "go", "test", "./...")
		if execErr != nil {
			return nil, fmt.Errorf("implement: test run (iter %d): %w", wfCtx.TryCount, execErr)
		}
		wfCtx.LastTestOutput = out

		if err = s.Logger.Info(
			"implement.test_result",
			fmt.Sprintf("試行 %d: success=%v\n%s", wfCtx.TryCount, success, out),
		); err != nil {
			return nil, err
		}

		if success {
			if err = s.Logger.Info("implement.done", "すべてのテストが通過しました (Green)"); err != nil {
				return nil, err
			}
			return s.Next, nil
		}
	}

	return nil, fmt.Errorf("implement: Fix Loop の上限 (%d 回) に達しました。最後のテスト出力:\n%s",
		maxTryCount, wfCtx.LastTestOutput)
}

func (s *ImplementState) generateTestCode(ctx context.Context, wfCtx *Context) (string, error) {
	prompt := fmt.Sprintf(
		"[TEST_GEN] 以下の実装計画に基づいてGoのテストコード(task_test.go)を生成してください。\n\n%s",
		wfCtx.PlanText,
	)
	resp, err := s.Coder.Ask(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("implement: generate test: %w", err)
	}

	code, ok := markdown.ExtractCodeBlock(resp, "go")
	if !ok {
		return "", fmt.Errorf("implement: LLM returned no Go code block for test")
	}
	return code, nil
}

func (s *ImplementState) generateImplCode(ctx context.Context, wfCtx *Context) (string, error) {
	prompt := fmt.Sprintf(
		"以下の実装計画とテスト失敗の出力を元に、プロダクトコードを生成してください。\n\n## 実装計画\n%s\n\n## テスト出力\n%s",
		wfCtx.PlanText,
		wfCtx.LastTestOutput,
	)
	resp, err := s.Coder.Ask(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("implement: generate impl: %w", err)
	}

	code, ok := markdown.ExtractCodeBlock(resp, "go")
	if !ok {
		return "", fmt.Errorf("implement: LLM returned no Go code block for implementation")
	}
	return code, nil
}
