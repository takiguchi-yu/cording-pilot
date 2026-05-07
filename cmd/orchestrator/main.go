// Command orchestrator は AI エージェントオーケストレーション CLI です。
//
// LLM クライアント、Executor、Agent Factory、各ワークフローステートの依存関係全体を配線し、
// エージェントシーケンス図に従った TDD スタイルのコード生成パイプラインを驱動します。
//
// 使い方:
//
//	orchestrator "<要件>"
//
// 使用例:
//
//	orchestrator "文字列を逆順にする関数"
//
// 実行ログはカレントディレクトリの run.ndjson に記録されます。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/takiguchi-yu/cording-pilot/internal/agent"
	"github.com/takiguchi-yu/cording-pilot/internal/config"
	"github.com/takiguchi-yu/cording-pilot/internal/executor"
	"github.com/takiguchi-yu/cording-pilot/internal/llm"
	"github.com/takiguchi-yu/cording-pilot/internal/workflow"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

func main() {
	useDocker := flag.Bool("docker", false, "ローカルの代わりに Docker Executor を使用する")
	dockerImage := flag.String("docker-image", "", "Docker Executor で使用するイメージ（省略時は設定ファイルの image を使用）")
	configPath := flag.String("config", config.DefaultConfigFileName, "プロジェクト設定ファイルのパス")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, `Usage: orchestrator [--docker] [--docker-image IMAGE] [--config PATH] "<requirement>"`)
		os.Exit(1)
	}
	requirement := flag.Arg(0)

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// --docker-image フラグが明示指定された場合は設定ファイルの値を上書きする。
	if *dockerImage != "" {
		cfg.Environment.Image = *dockerImage
	}

	var exec executor.Executor
	if *useDocker {
		exec = executor.NewDockerExecutor(cfg.Environment.Image)
	} else {
		exec = executor.NewLocalExecutor()
	}

	logFile, err := os.Create("run.ndjson")
	if err != nil {
		log.Fatalf("failed to create log file: %v", err)
	}
	defer func() {
		if closeErr := logFile.Close(); closeErr != nil {
			log.Printf("warning: failed to close log file: %v", closeErr)
		}
	}()

	if runErr := run(requirement, logFile, exec, cfg); runErr != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", runErr)
		os.Exit(1)
	}
}

// run は依存関係グラフ（DI コンテナー）を構築してワークフローを開始します。
// main から分離することで、注入ロジックをテストで独立して検証できるようにしています。
func run(requirement string, logDest *os.File, exec executor.Executor, cfg *config.Config) error {
	// ── Strategies ──────────────────────────────────────────────────────────
	log := logger.New(logDest)

	llmClient, err := newLLMClient(cfg, log)
	if err != nil {
		_ = log.Error("startup", fmt.Sprintf("LLM クライアントの初期化に失敗しました: %v", err))
		return fmt.Errorf("llm client: %w", err)
	}

	// ── Agent Factory ────────────────────────────────────────────────────────
	factory := agent.NewFactory(llmClient)
	planner := factory.NewPlannerAgent()
	coder := factory.NewCoderAgent()
	reviewer := factory.NewReviewer()

	// ── State Graph (wired bottom-up to avoid forward references) ────────────
	completeState := &workflow.CompleteState{
		Logger: log,
	}

	// ImplementState is referenced by both ReviewState (on rejection) and
	// PlanState (as the initial Next).  We create a shared pointer so that
	// ReviewState.OnReject points to the same node as PlanState.Next.
	implementState := &workflow.ImplementState{
		Coder:  coder,
		Exec:   exec,
		Logger: log,
		// Next is set after ReviewState is created to avoid a forward reference.
	}

	reviewState := &workflow.ReviewState{
		Reviewer:  reviewer,
		Logger:    log,
		OnApprove: completeState,
		OnReject:  implementState, // re-enter implementation on rejection
	}

	implementState.Next = reviewState // complete the cycle

	planState := &workflow.PlanState{
		Planner: planner,
		Logger:  log,
		Next:    implementState,
	}

	interactiveState := &workflow.InteractiveState{
		Planner: planner,
		Logger:  log,
		Next:    planState,
	}

	// ── Workflow Context ─────────────────────────────────────────────────────
	wfCtx := &workflow.Context{
		Requirement: requirement,
		Config:      cfg,
	}

	// ── Runner ───────────────────────────────────────────────────────────────
	runner := workflow.NewRunner()
	return runner.Run(context.Background(), interactiveState, wfCtx)
}

// newLLMClient は cfg.LLM.Provider に基づいて適切な llm.Client を生成します。
// 必要な環境変数が未設定の場合は起動時に Fail-fast します。
func newLLMClient(cfg *config.Config, log *logger.Logger) (llm.Client, error) {
	switch cfg.LLM.Provider {
	case "copilot":
		token := os.Getenv("GITHUB_TOKEN")
		if token == "" {
			return nil, fmt.Errorf("provider %q には GITHUB_TOKEN 環境変数が必要です", cfg.LLM.Provider)
		}
		return llm.NewCopilotClient(cfg.LLM.Model, token, log)
	default:
		return nil, fmt.Errorf("未対応の LLM プロバイダーです: %q (対応プロバイダー: copilot)", cfg.LLM.Provider)
	}
}
