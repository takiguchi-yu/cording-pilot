package workflow

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

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
	// maxTestRedRetries は Phase A でテストが Red になるまでの最大再試行回数です。
	maxTestRedRetries = 3
)

// ImplementState は ③ 実装フェーズ（Fix Loop 含む）です。
// TDD サイクル（テスト生成→実行（Red）→実装生成→実行（Green or Red））を最大 maxTryCount 回繰り返します。
// 成功時は Next（ReviewState）へ遷移し、上限到達時はエラーを返します。
type ImplementState struct {
	Coder  agent.CoderAgent
	Exec   executor.Executor
	Logger *logger.Logger
	// Supervisor は Fix Loop 迷走時に方閇転換のアドバイスを行うエージェントです。
	// nil の場合は Supervisor 機能を無効にします。
	Supervisor agent.SupervisorAgent
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
		fmt.Sprintf("Docker イメージ: %s, パイプライン: %d コマンド", cfg.Environment.Image, len(cfg.Pipeline.Check)),
	); err != nil {
		return nil, err
	}

	filteredPlanText := FilterIssueForCoder(wfCtx.PlanText)
	originalChars := utf8.RuneCountInString(strings.TrimSpace(wfCtx.PlanText))
	filteredChars := utf8.RuneCountInString(strings.TrimSpace(filteredPlanText))
	reductionPercent := 0.0
	if originalChars > 0 {
		reductionPercent = 100.0 * (1.0 - float64(filteredChars)/float64(originalChars))
	}
	if err := s.Logger.Debug(
		"implement.prompt_filter",
		fmt.Sprintf(
			"Issue コンテキスト軽量化: before=%d chars, after=%d chars, reduction=%.1f%%",
			originalChars,
			filteredChars,
			reductionPercent,
		),
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

	// Knowledge をプロンプトに注入する。
	knowledge := LoadKnowledge(s.Logger, repoRoot, cfg.Knowledge)
	if err = s.Logger.Debug("implement.knowledge", fmt.Sprintf("知識として %d 文字読み込みました", utf8.RuneCountInString(knowledge))); err != nil {
		return nil, err
	}
	if knowledge != "" {
		filteredPlanText = "## プロジェクトの前提知識・ルール (Project Knowledge)\n以下の知識やルールを最優先で遵守して計画・実装を行ってください。\n\n" + knowledge + "\n" + filteredPlanText
	}

	// Step 0: タスクをサブタスクに分割する（Decomposition）。
	subtasks, err := s.Coder.DecomposeTask(ctx, filteredPlanText)
	if err != nil {
		return nil, fmt.Errorf("implement: decompose task: %w", err)
	}
	if len(subtasks) == 0 {
		subtasks = []string{filteredPlanText}
	}
	if err = s.Logger.Info("implement.decompose",
		fmt.Sprintf("タスクを %d 個のサブタスクに分割しました", len(subtasks))); err != nil {
		return nil, err
	}

	// Step 1: 動的コンテキストを収集してハルシネーション抑制に役立てる（テスト生成前）。
	codeContext, contextFiles := gatherContext(workDir, filteredPlanText)
	if err = s.Logger.Info("implement.context",
		fmt.Sprintf("コードコンテキスト収集完了: %d ファイル読み込み: %v", len(contextFiles), contextFiles)); err != nil {
		return nil, err
	}

	// Step 2: Phase A – テストコードを生成し、Red（FAIL）を確認する。
	// テストが最初から Green になる場合は「失敗するテストを書け」と LLM にフィードバックして再試行する。
	testRedConfirmed := false
	for testRedRetry := 0; testRedRetry < maxTestRedRetries; testRedRetry++ {
		if err = s.Logger.Info("implement.phase_a",
			fmt.Sprintf("Phase A: テスト生成 (試行 %d/%d)", testRedRetry+1, maxTestRedRetries)); err != nil {
			return nil, err
		}

		testResult, testGenErr := s.generateTestCode(ctx, wfCtx, filteredPlanText)
		if testGenErr != nil {
			return nil, testGenErr
		}
		if len(testResult.Files) == 0 {
			return nil, fmt.Errorf("implement: LLM returned no files for test code generation")
		}
		for _, f := range testResult.Files {
			if writeErr := ApplyPatch(workDir, f); writeErr != nil {
				return nil, fmt.Errorf("implement: apply test patch: %w", writeErr)
			}
		}

		s.runAutoFix(ctx, workDir, cfg.Pipeline.AutoFix, s.Exec)

		initialOut, initialSuccess, initialErr := s.runPipeline(ctx, workDir, cfg.Pipeline.Check)
		if initialErr != nil {
			return nil, fmt.Errorf("implement: initial pipeline run: %w", initialErr)
		}
		wfCtx.LastTestOutput = TruncateLog(initialOut, 50)

		if !initialSuccess {
			// Red 確認 ✓ – TDD サイクル開始可能。
			if err = s.Logger.Info("implement.phase_a_red",
				fmt.Sprintf("Phase A: Red 確認 (試行 %d) – TDD サイクルを開始します\n%s", testRedRetry+1, initialOut)); err != nil {
				return nil, err
			}
			testRedConfirmed = true
			break
		}

		// Green になってしまった: テストが最初から通っている（意味のないテスト）。
		if err = s.Logger.Info("implement.phase_a_green_warning",
			fmt.Sprintf("Phase A: テストが初回から Green です。失敗するテストを再生成します (試行 %d/%d)", testRedRetry+1, maxTestRedRetries)); err != nil {
			return nil, err
		}
		wfCtx.LastTestOutput = "テストが最初から通っています（Green）。失敗するテストを書いてください。既存の実装に対して必ず失敗する、意味のあるテストを生成してください。"
	}

	if !testRedConfirmed {
		return nil, fmt.Errorf("implement: テストが %d 回試行しても Red になりませんでした。テスト生成を中断します", maxTestRedRetries)
	}

	// Step 3: Phase B – サブタスクごとに Fix Loop（Impl → Green）を実行する。
	for subtaskIdx, subtask := range subtasks {
		if err = s.Logger.Info("implement.subtask_start",
			fmt.Sprintf("サブタスク %d/%d 開始: %s", subtaskIdx+1, len(subtasks), subtask)); err != nil {
			return nil, err
		}

		implCache := make(map[string]agent.CodeGenerationResult)
		detector := &loopDetector{}
		supervisorAdvice := ""
		subtaskDone := false
		for localTry := 0; localTry < maxTryCount; localTry++ {
			wfCtx.TryCount++

			if err = s.Logger.Info("implement.fix_loop",
				fmt.Sprintf("サブタスク %d/%d Fix Loop 試行 %d/%d",
					subtaskIdx+1, len(subtasks), localTry+1, maxTryCount)); err != nil {
				return nil, err
			}

			cacheKey := filteredPlanText + "\n\n" + subtask + "\n\n" + wfCtx.LastTestOutput + "\n\n" + supervisorAdvice
			implResult, fromCache, genErr := s.generateImplCodeWithCache(
				ctx, wfCtx, filteredPlanText, subtask, codeContext, supervisorAdvice, cacheKey, implCache)
			if genErr != nil {
				// JSON パースエラーの場合はエラー内容を LLM にフィードバックして再試行する。
				if errors.Is(genErr, llm.ErrJSONParse) {
					wfCtx.LastTestOutput = genErr.Error()
					supervisorAdvice = ""
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
				supervisorAdvice = ""
				continue
			}
			if fromCache {
				if err = s.Logger.Info("implement.cache_hit", "同一条件のため実装生成結果をキャッシュから再利用します"); err != nil {
					return nil, err
				}
			}

			for _, f := range implResult.Files {
				if writeErr := ApplyPatch(workDir, f); writeErr != nil {
					// パッチ適用失敗: 「検索文字列が見つかりません」等を LLM にフィードバックする。
					wfCtx.LastTestOutput = writeErr.Error()
					supervisorAdvice = ""
					if logErr := s.Logger.Info("implement.patch_error",
						fmt.Sprintf("パッチ適用エラーを Fix Loop にフィードバックします: %v", writeErr)); logErr != nil {
						return nil, logErr
					}
					continue
				}
			}

			s.runAutoFix(ctx, workDir, cfg.Pipeline.AutoFix, s.Exec)

			out, success, pipeErr := s.runPipeline(ctx, workDir, cfg.Pipeline.Check)
			if pipeErr != nil {
				return nil, fmt.Errorf("implement: pipeline run (subtask %d, iter %d): %w",
					subtaskIdx+1, wfCtx.TryCount, pipeErr)
			}
			// Fix Loop の出力はスマートに切り詰めてトークン消費を抑制する。
			truncatedOut := TruncateLog(out, 50)
			wfCtx.LastTestOutput = truncatedOut

			if err = s.Logger.Info("implement.pipeline_result",
				fmt.Sprintf("サブタスク %d/%d 試行 %d: success=%v\n%s",
					subtaskIdx+1, len(subtasks), localTry+1, success, out)); err != nil {
				return nil, err
			}

			if success {
				if err = s.Logger.Info("implement.subtask_done",
					fmt.Sprintf("サブタスク %d/%d が完了しました (Green)", subtaskIdx+1, len(subtasks))); err != nil {
					return nil, err
				}
				subtaskDone = true
				break
			}

			// LoopDetector: 同じエラーが2回連続した場合は Supervisor に助言を求める。
			supervisorAdvice = ""
			if detector.isDuplicate(truncatedOut) && s.Supervisor != nil {
				if err = s.Logger.Info("implement.loop_detected",
					fmt.Sprintf("サブタスク %d/%d: 同一エラーが連続しました。Supervisor に助言を求めます", subtaskIdx+1, len(subtasks))); err != nil {
					return nil, err
				}
				advice, advErr := s.Supervisor.Advise(ctx, codeContext, fmt.Sprintf("試行 %d 回目", wfCtx.TryCount), truncatedOut)
				if advErr != nil {
					_ = s.Logger.Warn("implement.supervisor_error",
						fmt.Sprintf("Supervisor 呼び出しエラー（Fix Loop を継続します）: %v", advErr))
				} else {
					supervisorAdvice = advice
					if err = s.Logger.Info("implement.supervisor_advice",
						fmt.Sprintf("Supervisor の助言:\n%s", advice)); err != nil {
						return nil, err
					}
				}
			}
		} // end for localTry

		if !subtaskDone {
			return nil, fmt.Errorf("implement: サブタスク %d/%d の Fix Loop の上限 (%d 回) に達しました。最後のパイプライン出力:\n%s",
				subtaskIdx+1, len(subtasks), maxTryCount, wfCtx.LastTestOutput)
		}
	}

	if err = s.Logger.Info("implement.done", "すべてのパイプラインステップが通過しました (Green)"); err != nil {
		return nil, err
	}
	return s.Next, nil
}

// runPipeline は cfg.Pipeline の各ステップを順番に Executor で実行します。
// いずれかのステップが失敗した時点でループを中断し、それまでの全出力と false を返します。
// インフラエラー（Executor 自体の障害）は error として伝播します。
// 全ステップが成功した場合は全出力と true を返します。
// runPipeline は cfg.Pipeline.Check の各コマンドを順番に Executor で実行します。
// いずれかのコマンドが失敗した時点でループを中断し、それまでの全出力と false を返します。
// インフラエラー（Executor 自体の障害）は error として伝播します。
// 全コマンドが成功した場合は全出力と true を返します。
// コマンドは "sh" "-c" 経由で実行するため、パイプ (|) やリダイレクトなどのシェル構文も使用できます。
func (s *ImplementState) runPipeline(ctx context.Context, workDir string, cmds []string) (string, bool, error) {
	var sb strings.Builder
	for _, cmdStr := range cmds {
		if strings.TrimSpace(cmdStr) == "" {
			continue
		}
		_ = s.Logger.Info("implement.pipeline_step", fmt.Sprintf("ステップ実行: %s", cmdStr))

		out, success, execErr := s.Exec.Run(ctx, workDir, "sh", "-c", cmdStr)
		fmt.Fprintf(&sb, "=== %s ===\n%s\n", cmdStr, out)
		if execErr != nil {
			fmt.Fprintf(&sb, "`%s` 実行時のエラー\n", cmdStr)
			return sb.String(), false, fmt.Errorf("`%s` 実行時のエラー: %w", cmdStr, execErr)
		}
		if !success {
			fmt.Fprintf(&sb, "`%s` が失敗しました\n", cmdStr)
			return sb.String(), false, nil
		}
	}
	return sb.String(), true, nil
}

// runAutoFix は pipeline.auto_fix の各コマンドを順番に Executor で実行します。
// いずれかのコマンドが失敗した場合でも、処理を続行（エラーを返さず、ログに出力）します。
// 最終的な合否判定は後続の runPipeline に委ねられます。
// コマンドは "sh" "-c" 経由で実行するため、パイプなどのシェル構文も使用できます。
func (s *ImplementState) runAutoFix(
	ctx context.Context,
	workDir string,
	cmds []string,
	exec executor.Executor,
) {
	if len(cmds) == 0 {
		return
	}

	if err := s.Logger.Info("implement.auto_fix_start", "自己修復フェーズを開始します"); err != nil {
		// ログ出力エラーは無視して続行
		return
	}

	for _, cmdStr := range cmds {
		if strings.TrimSpace(cmdStr) == "" {
			continue
		}

		out, success, execErr := exec.Run(ctx, workDir, "sh", "-c", cmdStr)
		if execErr != nil {
			// インフラエラーでもログに出力するが処理は続行
			_ = s.Logger.Warn(
				"implement.auto_fix_error",
				fmt.Sprintf("`%s` 実行時のエラー: %v\nOutput:\n%s", cmdStr, execErr, out),
			)
			continue
		}
		if !success {
			// ステップが失敗しても、エラーではなく info として出力
			_ = s.Logger.Info(
				"implement.auto_fix_step_failed",
				fmt.Sprintf("`%s` が失敗しましたが処理を続行します (最終判定は pipeline に委ねます)\n%s", cmdStr, out),
			)
			continue
		}
		// 成功時もログ出力
		_ = s.Logger.Debug(
			"implement.auto_fix_step_success",
			fmt.Sprintf("`%s` が成功しました", cmdStr),
		)
	}

	if err := s.Logger.Info("implement.auto_fix_end", "自己修復フェーズが完了しました (結果は pipeline で最終判定されます)"); err != nil {
		// ログ出力エラーは無視して続行
		return
	}
}

func (s *ImplementState) generateTestCode(ctx context.Context, wfCtx *Context, planText string) (agent.CodeGenerationResult, error) {
	cfg := wfCtx.Config
	if cfg == nil {
		cfg = config.DefaultGoConfig()
	}
	prompt := fmt.Sprintf(
		"以下の実装計画に基づいて [%s] のテストコードを生成してください。ファイルの拡張子やディレクトリ構造は対象言語のベストプラクティスおよび既存のリポジトリ構成に従うこと。\n\n%s",
		cfg.Project.Language,
		planText,
	)
	if envHeader := BuildProjectEnvHeader(cfg); envHeader != "" {
		prompt = envHeader + "\n\n" + prompt
	}
	if wfCtx.LastTestOutput != "" {
		prompt = fmt.Sprintf("%s\n\n## 前回のフィードバック\n%s", prompt, wfCtx.LastTestOutput)
	}
	result, err := s.Coder.GenerateTest(ctx, prompt)
	if err != nil {
		return agent.CodeGenerationResult{}, fmt.Errorf("implement: generate test: %w", err)
	}
	return result, nil
}

func (s *ImplementState) generateImplCode(ctx context.Context, wfCtx *Context, planText string, subtask string, codeContext string, supervisorAdvice string) (agent.CodeGenerationResult, error) {
	cfg := wfCtx.Config
	if cfg == nil {
		cfg = config.DefaultGoConfig()
	}
	planForPrompt := compactPromptText(planText, maxPlanPromptChars)
	failureOutputForPrompt := compactPromptText(wfCtx.LastTestOutput, maxFailureOutputChars)
	prompt := fmt.Sprintf(
		"以下の実装計画とパイプライン失敗の出力を元に、[%s] のプロダクトコードを生成してください。ファイルの拡張子やディレクトリ構造は対象言語のベストプラクティスおよび既存のリポジトリ構成に従うこと。\n\n## 実装計画\n%s\n\n## 現在のサブタスク（優先的に実装すること）\n%s\n\n## パイプライン出力\n%s\n\n## 既存の実装コンテキスト\n%s",
		cfg.Project.Language,
		planForPrompt,
		subtask,
		failureOutputForPrompt,
		codeContext,
	)
	if supervisorAdvice != "" {
		prompt = fmt.Sprintf("以下の助言に従って別のアプローチを試してください。\n\n## Supervisor の助言\n%s\n\n---\n\n%s", supervisorAdvice, prompt)
	}
	result, err := s.Coder.GenerateImpl(ctx, prompt)
	if err != nil {
		return agent.CodeGenerationResult{}, fmt.Errorf("implement: generate impl: %w", err)
	}
	return result, nil
}

func (s *ImplementState) generateImplCodeWithCache(
	ctx context.Context,
	wfCtx *Context,
	planText string,
	subtask string,
	codeContext string,
	supervisorAdvice string,
	cacheKey string,
	cache map[string]agent.CodeGenerationResult,
) (agent.CodeGenerationResult, bool, error) {
	if result, ok := cache[cacheKey]; ok {
		return result, true, nil
	}

	result, err := s.generateImplCode(ctx, wfCtx, planText, subtask, codeContext, supervisorAdvice)
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

// goFilePathRe はプラン文字列から Go ソースファイルパスを抽出するための正規表現です。
var goFilePathRe = regexp.MustCompile(`[a-zA-Z0-9_./-]+\.go`)

// gatherContext は workDir のディレクトリ構造（2階層まで）と、
// plan から抽出した関連 Go ファイルの内容をまとめた文字列を返します。
// 第2戻り値は実際に読み込んだファイルパスのリストです。
func gatherContext(workDir string, plan string) (string, []string) {
	var sb strings.Builder

	// ディレクトリツリー（2階層目まで）を収集する。
	sb.WriteString("## ディレクトリ構造\n```\n")
	_ = filepath.WalkDir(workDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(workDir, path)
		if relErr != nil || rel == "." {
			return nil
		}
		base := filepath.Base(path)
		// 隠しディレクトリ／ファイルはスキップする。
		if strings.HasPrefix(base, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		depth := strings.Count(rel, string(filepath.Separator))
		if depth >= 2 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		indent := strings.Repeat("  ", depth)
		if d.IsDir() {
			fmt.Fprintf(&sb, "%s%s/\n", indent, base)
		} else {
			fmt.Fprintf(&sb, "%s%s\n", indent, base)
		}
		return nil
	})
	sb.WriteString("```\n\n")

	// plan からファイルパス文字列を抽出し、workDir 内に実在するものを読み込む。
	matches := goFilePathRe.FindAllString(plan, -1)
	seen := make(map[string]bool)
	var readFiles []string
	for _, m := range matches {
		if seen[m] {
			continue
		}
		seen[m] = true
		clean := filepath.Clean(m)
		// パストラバーサル防止。
		if strings.HasPrefix(clean, "..") {
			continue
		}
		fullPath := filepath.Join(workDir, clean)
		rel, relErr := filepath.Rel(workDir, fullPath)
		if relErr != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		content, readErr := os.ReadFile(fullPath)
		if readErr != nil {
			continue
		}
		readFiles = append(readFiles, m)
		fmt.Fprintf(&sb, "## %s\n```go\n%s\n```\n\n", m, string(content))
	}

	return sb.String(), readFiles
}

// loopDetector は Fix Loop 内で同一エラーの繰り返しを検知します。
// isDuplicate を連続して呼ぶことで「前回と同じエラーが2回続いた」状態を検出できます。
type loopDetector struct {
	prev string
}

// isDuplicate は currentErr が前回と同じ場合に true を返し、内部状態を更新します。
// 初回呼び出しは常に false を返します。
func (d *loopDetector) isDuplicate(currentErr string) bool {
	dup := d.prev != "" && d.prev == currentErr
	d.prev = currentErr
	return dup
}
