// Command orchestrator は AI エージェントオーケストレーション CLI です。
//
// LLM クライアント、Executor、Agent Factory、各ワークフローステートの依存関係全体を配線し、
// エージェントシーケンス図に従った TDD スタイルのコード生成パイプラインを驱動します。
//
// 使い方:
//
//	orchestrator "<要件>"
//	orchestrator --issue 12
//	orchestrator https://github.com/owner/repo/issues/42
//
// 使用例:
//
//	orchestrator "文字列を逆順にする関数"
//	orchestrator --issue 42
//	orchestrator https://github.com/takiguchi-yu/cording-pilot/issues/42
//
// 実行ログはカレントディレクトリの run.ndjson に記録されます。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/takiguchi-yu/cording-pilot/internal/agent"
	"github.com/takiguchi-yu/cording-pilot/internal/config"
	"github.com/takiguchi-yu/cording-pilot/internal/executor"
	githubpkg "github.com/takiguchi-yu/cording-pilot/internal/github"
	"github.com/takiguchi-yu/cording-pilot/internal/llm"
	"github.com/takiguchi-yu/cording-pilot/internal/tui"
	"github.com/takiguchi-yu/cording-pilot/internal/workflow"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
	"github.com/takiguchi-yu/cording-pilot/pkg/retry"
)

func main() {
	useDocker := flag.Bool("docker", false, "ローカルの代わりに Docker Executor を使用する（environment.type=docker のショートカット）")
	useNix := flag.Bool("nix", false, "ローカルの代わりに Nix Executor を使用する（environment.type=nix のショートカット）")
	dockerImage := flag.String("docker-image", "", "Docker Executor で使用するイメージ（省略時は設定ファイルの image を使用）")
	configPath := flag.String("config", config.DefaultConfigFileName, "プロジェクト設定ファイルのパス")
	issueNumber := flag.Int("issue", 0, "処理する GitHub Issue 番号（指定時は要件引数を省略可）")
	flag.Parse()

	if flag.NArg() < 1 && *issueNumber == 0 {
		fmt.Fprintln(os.Stderr, `Usage: orchestrator [--docker] [--nix] [--docker-image IMAGE] [--config PATH] [--issue NUMBER] "<requirement>"|<issue-url>`)
		os.Exit(1)
	}

	// positional 引数が GitHub Issue URL の場合は owner/repo/番号を自動解析する。
	requirement := ""
	issueURLOwner := ""
	issueURLRepo := ""
	if flag.NArg() > 0 {
		arg := flag.Arg(0)
		if ref, err := githubpkg.ParseIssueURL(arg); err == nil {
			*issueNumber = ref.Number
			issueURLOwner = ref.Owner
			issueURLRepo = ref.Repo
		} else {
			requirement = arg
		}
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// --docker-image フラグが明示指定された場合は設定ファイルの値を上書きする。
	if *dockerImage != "" {
		cfg.Environment.Image = *dockerImage
	}

	// CLI フラグは設定ファイルの environment.type を上書きする。
	executorType := cfg.Environment.Type
	if *useNix {
		executorType = "nix"
	} else if *useDocker {
		executorType = "docker"
	}

	var exec executor.Executor
	switch executorType {
	case "nix":
		nixExec, nixErr := executor.NewNixExecutor()
		if nixErr != nil {
			log.Fatalf("failed to create nix executor: %v", nixErr)
		}
		exec = nixExec
	case "docker":
		exec = executor.NewDockerExecutor(cfg.Environment.Image)
	default:
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

	if runErr := run(requirement, *issueNumber, issueURLOwner, issueURLRepo, logFile, exec, cfg); runErr != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", runErr)
		os.Exit(1)
	}
}

// run は依存関係グラフ（DI コンテナー）を構築してワークフローを開始します。
// main から分離することで、注入ロジックをテストで独立して検証できるようにしています。
// issueURLOwner / issueURLRepo が非空の場合は、git リモートから検出した値を上書きします。
func run(requirement string, issueNumber int, issueURLOwner, issueURLRepo string, logDest *os.File, exec executor.Executor, cfg *config.Config) error {
	// ── Strategies ──────────────────────────────────────────────────────────
	log := logger.New(logDest)

	llmClient, err := newLLMClient(cfg, log)
	if err != nil {
		_ = log.Error("startup", fmt.Sprintf("LLM クライアントの初期化に失敗しました: %v", err))
		return fmt.Errorf("llm client: %w", err)
	}
	plannerLLMClient, err := newPlannerLLMClient(cfg, log)
	if err != nil {
		_ = log.Error("startup", fmt.Sprintf("Planner LLM クライアントの初期化に失敗しました: %v", err))
		return fmt.Errorf("planner llm client: %w", err)
	}

	// ── GitHub クライアント（オプション） ──────────────────────────────────────
	ghClient, ghToken, repoOwner, repoName, baseBranch := initGitHub(context.Background(), log)

	// Issue URL から解析した owner/repo が指定されている場合は上書きする。
	if issueURLOwner != "" {
		repoOwner = issueURLOwner
	}
	if issueURLRepo != "" {
		repoName = issueURLRepo
	}

	// 自然言語入力のみで owner/repo を自動判定できない場合は、TUI で入力を受け付ける。
	if ghClient != nil && requirement != "" && issueURLOwner == "" && issueURLRepo == "" {
		if strings.TrimSpace(repoOwner) == "" || strings.TrimSpace(repoName) == "" {
			if infoErr := log.Info("startup.github", "owner/repo を自動判定できなかったため、TUI で入力を受け付けます"); infoErr != nil {
				return infoErr
			}

			inputOwner, inputRepo, inputErr := tui.RunRepoInput(repoOwner, repoName)
			if inputErr != nil {
				if errors.Is(inputErr, tui.ErrAborted) {
					return fmt.Errorf("run: %w", tui.ErrAborted)
				}
				return fmt.Errorf("run: input owner/repo: %w", inputErr)
			}

			repoOwner = inputOwner
			repoName = inputRepo
		}
	}

	// ── Agent Factory ────────────────────────────────────────────────────────
	factory := agent.NewFactory(llmClient, cfg)
	planner := factory.NewPlannerAgent()
	coder := factory.NewCoderAgent()
	reviewer := factory.NewReviewer()

	// ── State Graph (wired bottom-up to avoid forward references) ────────────
	completeState := &workflow.CompleteState{
		Logger:      log,
		LLMClient:   plannerLLMClient,
		GitHub:      ghClient,
		GitHubToken: ghToken,
		RepoOwner:   repoOwner,
		RepoName:    repoName,
		BaseBranch:  baseBranch,
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
		Planner:   planner,
		Logger:    log,
		Next:      planState,
		GitHub:    ghClient,
		RepoOwner: repoOwner,
		RepoName:  repoName,
	}

	// ── Workflow Context ─────────────────────────────────────────────────────
	wfCtx := &workflow.Context{
		Requirement: requirement,
		Config:      cfg,
		IssueNumber: issueNumber,
	}

	// ── Runner ───────────────────────────────────────────────────────────────
	runner := workflow.NewRunner()
	runCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	return runner.Run(runCtx, interactiveState, wfCtx)
}

// newLLMClient は cfg.LLM の設定に基づいてエージェント別の LLM クライアントを生成し、
// RoutingClient としてラップして返します。
// エージェントごとに異なるプロバイダーとモデルを使用するハイブリッド構成をサポートします。
func newLLMClient(cfg *config.Config, log *logger.Logger) (llm.Client, error) {
	retryPolicy := retry.Policy{
		MaxAttempts:  cfg.LLM.Retry.Attempts,
		InitialDelay: time.Duration(cfg.LLM.Retry.InitialDelayMS) * time.Millisecond,
		Multiplier:   cfg.LLM.Retry.Multiplier,
	}
	opts := llm.CopilotOptions{
		RetryPolicy:      retryPolicy,
		RateLimitMode:    cfg.LLM.RateLimit.Mode,
		MaxRateLimitWait: time.Duration(cfg.LLM.RateLimit.MaxWaitSeconds) * time.Second,
	}

	defaultClient, err := llm.NewClient(cfg.LLM.Default, log, opts)
	if err != nil {
		return nil, fmt.Errorf("llm default client: %w", err)
	}

	plannerClient, err := llm.NewClient(cfg.LLM.GetPlannerConfig(), log, opts)
	if err != nil {
		return nil, fmt.Errorf("llm planner client: %w", err)
	}

	coderClient, err := llm.NewClient(cfg.LLM.GetCoderConfig(), log, opts)
	if err != nil {
		return nil, fmt.Errorf("llm coder client: %w", err)
	}

	reviewerClient, err := llm.NewClient(cfg.LLM.GetReviewerConfig(), log, opts)
	if err != nil {
		return nil, fmt.Errorf("llm reviewer client: %w", err)
	}

	return llm.NewRoutingClient(defaultClient, llm.RoleClients{
		PlannerClarification: plannerClient,
		PlannerPlan:          plannerClient,
		Coder:                coderClient,
		Reviewer:             reviewerClient,
	}), nil
}

// newPlannerLLMClient は Planner 用設定の LLM クライアントを生成します。
func newPlannerLLMClient(cfg *config.Config, log *logger.Logger) (llm.Client, error) {
	retryPolicy := retry.Policy{
		MaxAttempts:  cfg.LLM.Retry.Attempts,
		InitialDelay: time.Duration(cfg.LLM.Retry.InitialDelayMS) * time.Millisecond,
		Multiplier:   cfg.LLM.Retry.Multiplier,
	}
	opts := llm.CopilotOptions{
		RetryPolicy:      retryPolicy,
		RateLimitMode:    cfg.LLM.RateLimit.Mode,
		MaxRateLimitWait: time.Duration(cfg.LLM.RateLimit.MaxWaitSeconds) * time.Second,
	}

	plannerClient, err := llm.NewClient(cfg.LLM.GetPlannerConfig(), log, opts)
	if err != nil {
		return nil, fmt.Errorf("llm planner client: %w", err)
	}

	return plannerClient, nil
}

// initGitHub は GITHUB_TOKEN を用いて GitHub クライアントとリポジトリ情報を初期化します。
// GITHUB_TOKEN が未設定の場合は nil と空文字を返し、GitHub 連携をスキップします。
func initGitHub(ctx context.Context, log *logger.Logger) (
	client githubpkg.Client,
	token, owner, repo, baseBranch string,
) {
	token = os.Getenv("GITHUB_TOKEN")
	if token == "" {
		_ = log.Info("startup.github", "GITHUB_TOKEN が未設定のため GitHub 連携をスキップします")
		return nil, "", "", "", ""
	}

	owner, repo, err := githubpkg.GetRepoInfo()
	if err != nil {
		_ = log.Info("startup.github", fmt.Sprintf("Git リポジトリ情報の取得に失敗しました。GitHub 連携を無効化します: %v", err))
		return nil, "", "", "", ""
	}

	base, err := githubpkg.DetectBaseBranch(ctx)
	if err != nil {
		base = "main"
	}

	return githubpkg.NewGitHubClient(token), token, owner, repo, base
}
