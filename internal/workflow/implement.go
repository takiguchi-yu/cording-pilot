package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/takiguchi-yu/cording-pilot/internal/agent"
	"github.com/takiguchi-yu/cording-pilot/internal/config"
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

	// 設定が注入されていない場合はデフォルトを使用する。
	cfg := wfCtx.Config
	if cfg == nil {
		cfg = config.DefaultGoConfig()
	}

	if err := s.Logger.Info(
		"implement.config",
		fmt.Sprintf("Docker イメージ: %s, パイプライン: %d ステップ", cfg.Environment.Image, len(cfg.Pipeline)),
	); err != nil {
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

	// Step 2: initial pipeline run (expected Red — no implementation yet).
	initialOut, _, initialErr := s.runPipeline(ctx, workDir, cfg.Pipeline)
	if initialErr != nil {
		return nil, fmt.Errorf("implement: initial pipeline run: %w", initialErr)
	}
	wfCtx.LastTestOutput = initialOut
	if err = s.Logger.Info("implement.initial_pipeline", fmt.Sprintf("初期パイプライン実行 (Red 想定)\n%s", initialOut)); err != nil {
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

		out, success, pipeErr := s.runPipeline(ctx, workDir, cfg.Pipeline)
		if pipeErr != nil {
			return nil, fmt.Errorf("implement: pipeline run (iter %d): %w", wfCtx.TryCount, pipeErr)
		}
		wfCtx.LastTestOutput = out

		if err = s.Logger.Info(
			"implement.pipeline_result",
			fmt.Sprintf("試行 %d: success=%v\n%s", wfCtx.TryCount, success, out),
		); err != nil {
			return nil, err
		}

		if success {
			if err = s.Logger.Info("implement.done", "すべてのパイプラインステップが通過しました (Green)"); err != nil {
				return nil, err
			}
			return s.Next, nil
		}
	}

	return nil, fmt.Errorf("implement: Fix Loop の上限 (%d 回) に達しました。最後のパイプライン出力:\n%s",
		maxTryCount, wfCtx.LastTestOutput)
}

// runPipeline は cfg.Pipeline の各ステップを順番に Executor で実行します。
// いずれかのステップが失敗した時点でループを中断し、それまでの全出力と false を返します。
// インフラエラー（Executor 自体の障害）は error として伝播します。
// 全ステップが成功した場合は全出力と true を返します。
func (s *ImplementState) runPipeline(ctx context.Context, workDir string, steps []config.PipelineStep) (string, bool, error) {
	var sb strings.Builder
	for _, step := range steps {
		_ = s.Logger.Info("implement.pipeline_step", fmt.Sprintf("ステップ実行: %s → %s", step.Name, step.Command))

		parts := strings.Fields(step.Command)
		if len(parts) == 0 {
			continue
		}
		cmd, args := parts[0], parts[1:]

		out, success, execErr := s.Exec.Run(ctx, workDir, cmd, args...)
		fmt.Fprintf(&sb, "=== %s: %s ===\n%s\n", step.Name, step.Command, out)
		if execErr != nil {
			return sb.String(), false, fmt.Errorf("step %q: %w", step.Name, execErr)
		}
		if !success {
			return sb.String(), false, nil
		}
	}
	return sb.String(), true, nil
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
		"以下の実装計画とパイプライン失敗の出力を元に、プロダクトコードを生成してください。\n\n## 実装計画\n%s\n\n## パイプライン出力\n%s",
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
