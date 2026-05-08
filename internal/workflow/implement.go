package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/takiguchi-yu/cording-pilot/internal/agent"
	"github.com/takiguchi-yu/cording-pilot/internal/config"
	"github.com/takiguchi-yu/cording-pilot/internal/executor"
	"github.com/takiguchi-yu/cording-pilot/internal/llm"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

const (
	maxTryCount           = 3
	maxPlanPromptChars    = 2500
	maxFailureOutputChars = 3000
)

// ImplementState は ② 実装フェーズ（Fix Loop 含む）です。
// TDD サイクル（テスト生成→実行（Red）→実装生成→実行（Green or Red））を最大 maxTryCount 回繰り返します。
// 成功時は Next（ReviewState）へ遷移し、上限到達時はエラーを返します。
type ImplementState struct {
	Coder  agent.CoderAgent
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

	// Set up the isolated working directory as a snapshot of the current repository.
	workDir, err := os.MkdirTemp("", "cording-pilot-*")
	if err != nil {
		return nil, fmt.Errorf("implement: create work dir: %w", err)
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("implement: get repository root: %w", err)
	}
	if err = copyDir(repoRoot, workDir); err != nil {
		return nil, fmt.Errorf("implement: snapshot repository: %w", err)
	}
	wfCtx.WorkDir = workDir

	// Step 1: generate test code.
	testResult, err := s.generateTestCode(ctx, wfCtx)
	if err != nil {
		return nil, err
	}
	if len(testResult.Files) == 0 {
		return nil, fmt.Errorf("implement: LLM returned no files for test code generation")
	}
	for _, f := range testResult.Files {
		if writeErr := safeWriteFile(workDir, f.Path, []byte(f.Content)); writeErr != nil {
			return nil, fmt.Errorf("implement: write test file: %w", writeErr)
		}
	}

	// Step 2a: auto_fix before initial pipeline run
	s.runAutoFix(ctx, workDir, cfg)

	// Step 2b: initial pipeline run (expected Red — no implementation yet).
	initialOut, _, initialErr := s.runPipeline(ctx, workDir, cfg.Pipeline)
	if initialErr != nil {
		return nil, fmt.Errorf("implement: initial pipeline run: %w", initialErr)
	}
	wfCtx.LastTestOutput = initialOut
	if err = s.Logger.Info("implement.initial_pipeline", fmt.Sprintf("初期パイプライン実行 (Red 想定)\n%s", initialOut)); err != nil {
		return nil, err
	}

	// Step 3: Fix Loop.
	implCache := make(map[string]agent.CodeGenerationResult)
	for wfCtx.TryCount < maxTryCount {
		wfCtx.TryCount++

		if err = s.Logger.Info(
			"implement.fix_loop",
			fmt.Sprintf("Fix Loop 試行 %d/%d", wfCtx.TryCount, maxTryCount),
		); err != nil {
			return nil, err
		}

		cacheKey := wfCtx.PlanText + "\n\n" + wfCtx.LastTestOutput
		implResult, fromCache, genErr := s.generateImplCodeWithCache(ctx, wfCtx, cacheKey, implCache)
		if genErr != nil {
			// JSON パースエラーの場合はエラー内容を LLM にフィードバックして再試行する。
			if errors.Is(genErr, llm.ErrJSONParse) {
				wfCtx.LastTestOutput = genErr.Error()
				if logErr := s.Logger.Info("implement.json_parse_error",
					fmt.Sprintf("JSON 解析エラーを Fix Loop にフィードバックします: %v", genErr)); logErr != nil {
					return nil, logErr
				}
				continue
			}
			return nil, genErr
		}
		if len(implResult.Files) == 0 {
			wfCtx.LastTestOutput = "実装コードのファイルリストが空です"
			continue
		}
		if fromCache {
			if err = s.Logger.Info("implement.cache_hit", "同一条件のため実装生成結果をキャッシュから再利用します"); err != nil {
				return nil, err
			}
		}

		for _, f := range implResult.Files {
			if writeErr := safeWriteFile(workDir, f.Path, []byte(f.Content)); writeErr != nil {
				return nil, fmt.Errorf("implement: write impl file: %w", writeErr)
			}
		}

		// Auto-fix before running pipeline
		s.runAutoFix(ctx, workDir, cfg)

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

// safeWriteFile は workDir 配下の relPath にコンテンツを書き込みます。
// パストラバーサル攻撃を防ぐため、書き込み先が workDir 内に収まることを検証します。
// "../" 等の不正なパスが指定された場合はエラーを返し、書き込みは行いません。
func safeWriteFile(workDir, relPath string, content []byte) error {
	if filepath.IsAbs(relPath) {
		return fmt.Errorf("safeWriteFile: 絶対パスは許可されていません: %q", relPath)
	}

	clean := filepath.Clean(relPath)
	// filepath.Clean 後でも ".." で始まる場合はパストラバーサルとみなす。
	if strings.HasPrefix(clean, "..") {
		return fmt.Errorf("safeWriteFile: パストラバーサルが検出されました: %q", relPath)
	}

	fullPath := filepath.Join(workDir, clean)
	// Rel で再検証: workDir の外を指す場合は ".." で始まる相対パスになる。
	rel, err := filepath.Rel(workDir, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("safeWriteFile: ワークディレクトリ外へのパスは許可されていません: %q", relPath)
	}

	if err = os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		return fmt.Errorf("safeWriteFile: ディレクトリ作成に失敗しました: %w", err)
	}
	return os.WriteFile(fullPath, content, 0o600)
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

// runAutoFix は auto_fix ステップを順番に Executor で実行します。
// いずれかのステップが失敗した場合でも、処理を続行（エラーを返さず、ログに出力）します。
// 最終的な合否判定は後続の runPipeline に委ねられます。
// cfg.LLM.AutoFixModel により、auto-fix フェーズで使用するモデルを指定できます。
func (s *ImplementState) runAutoFix(ctx context.Context, workDir string, cfg *config.Config) {
	if cfg == nil || len(cfg.AutoFix) == 0 {
		return
	}

	if err := s.Logger.Info("implement.auto_fix_start",
		fmt.Sprintf("自己修復フェーズを開始します (モデル: %s)", cfg.LLM.AutoFixModel)); err != nil {
		// ログ出力エラーは無視して続行
		return
	}

	for _, step := range cfg.AutoFix {
		parts := strings.Fields(step.Command)
		if len(parts) == 0 {
			continue
		}
		cmd, args := parts[0], parts[1:]

		out, success, execErr := s.Exec.Run(ctx, workDir, cmd, args...)
		if execErr != nil {
			// インフラエラーでもログに出力するが処理は続行
			_ = s.Logger.Warn(
				"implement.auto_fix_error",
				fmt.Sprintf("auto_fix ステップ %q でインフラエラーが発生しましたが処理を続行します: %v\nOutput:\n%s", step.Name, execErr, out),
			)
			continue
		}
		if !success {
			// ステップが失敗しても、エラーではなく info として出力
			_ = s.Logger.Info(
				"implement.auto_fix_step_failed",
				fmt.Sprintf("auto_fix ステップ %q が失敗しましたが処理を続行します (最終判定は pipeline に委ねます)\n%s", step.Name, out),
			)
			continue
		}
		// 成功時もログ出力
		_ = s.Logger.Debug(
			"implement.auto_fix_step_success",
			fmt.Sprintf("auto_fix ステップ %q が成功しました", step.Name),
		)
	}

	if err := s.Logger.Info("implement.auto_fix_end", "自己修復フェーズが完了しました (結果は pipeline で最終判定されます)"); err != nil {
		// ログ出力エラーは無視して続行
		return
	}
}

func (s *ImplementState) generateTestCode(ctx context.Context, wfCtx *Context) (agent.CodeGenerationResult, error) {
	cfg := wfCtx.Config
	if cfg == nil {
		cfg = config.DefaultGoConfig()
	}
	prompt := fmt.Sprintf(
		"[TEST_GEN] 以下の実装計画に基づいて [%s] のテストコードを [%s] を用いて生成してください。ファイルの拡張子やディレクトリ構造は対象言語のベストプラクティスおよび既存のリポジトリ構成に従うこと。\n\n%s",
		cfg.Project.Language,
		cfg.Project.TestFramework,
		wfCtx.PlanText,
	)
	result, err := s.Coder.GenerateCode(ctx, prompt)
	if err != nil {
		return agent.CodeGenerationResult{}, fmt.Errorf("implement: generate test: %w", err)
	}
	return result, nil
}

func (s *ImplementState) generateImplCode(ctx context.Context, wfCtx *Context) (agent.CodeGenerationResult, error) {
	cfg := wfCtx.Config
	if cfg == nil {
		cfg = config.DefaultGoConfig()
	}
	planForPrompt := compactPromptText(wfCtx.PlanText, maxPlanPromptChars)
	failureOutputForPrompt := compactPromptText(wfCtx.LastTestOutput, maxFailureOutputChars)
	prompt := fmt.Sprintf(
		"以下の実装計画とパイプライン失敗の出力を元に、[%s] のプロダクトコードを生成してください。ファイルの拡張子やディレクトリ構造は対象言語のベストプラクティスおよび既存のリポジトリ構成に従うこと。\n\n## 実装計画\n%s\n\n## パイプライン出力\n%s",
		cfg.Project.Language,
		planForPrompt,
		failureOutputForPrompt,
	)
	result, err := s.Coder.GenerateCode(ctx, prompt)
	if err != nil {
		return agent.CodeGenerationResult{}, fmt.Errorf("implement: generate impl: %w", err)
	}
	return result, nil
}

func (s *ImplementState) generateImplCodeWithCache(
	ctx context.Context,
	wfCtx *Context,
	cacheKey string,
	cache map[string]agent.CodeGenerationResult,
) (agent.CodeGenerationResult, bool, error) {
	if result, ok := cache[cacheKey]; ok {
		return result, true, nil
	}

	result, err := s.generateImplCode(ctx, wfCtx)
	if err != nil {
		return agent.CodeGenerationResult{}, false, err
	}
	cache[cacheKey] = result
	return result, false, nil
}

func compactPromptText(text string, maxChars int) string {
	trimmed := strings.TrimSpace(text)
	if maxChars <= 0 || len([]rune(trimmed)) <= maxChars {
		return trimmed
	}

	runes := []rune(trimmed)
	headChars := maxChars / 2
	tailChars := maxChars - headChars
	if headChars < 1 {
		headChars = 1
	}
	if tailChars < 1 {
		tailChars = 1
	}

	head := strings.TrimSpace(string(runes[:headChars]))
	tail := strings.TrimSpace(string(runes[len(runes)-tailChars:]))
	removed := len(runes) - headChars - tailChars
	if removed < 0 {
		removed = 0
	}

	return fmt.Sprintf("%s\n\n... [truncated %d chars] ...\n\n%s", head, removed, tail)
}
