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
	"fmt"
	"log"
	"os"

	"github.com/takiguchi-yu/cording-pilot/internal/agent"
	"github.com/takiguchi-yu/cording-pilot/internal/executor"
	"github.com/takiguchi-yu/cording-pilot/internal/llm"
	"github.com/takiguchi-yu/cording-pilot/internal/workflow"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, `Usage: orchestrator "<requirement>"`)
		os.Exit(1)
	}
	requirement := os.Args[1]

	logFile, err := os.Create("run.ndjson")
	if err != nil {
		log.Fatalf("failed to create log file: %v", err)
	}
	defer func() {
		if closeErr := logFile.Close(); closeErr != nil {
			log.Printf("warning: failed to close log file: %v", closeErr)
		}
	}()

	if runErr := run(requirement, logFile); runErr != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", runErr)
		os.Exit(1)
	}
}

// run は依存関係グラフ（DI コンテナー）を構築してワークフローを開始します。
// main から分離することで、注入ロジックをテストで独立して検証できるようにしています。
func run(requirement string, logDest *os.File) error {
	// ── Strategies ──────────────────────────────────────────────────────────
	llmClient := llm.NewMockClient()
	exec := executor.NewLocalExecutor()
	log := logger.New(logDest)

	// ── Agent Factory ────────────────────────────────────────────────────────
	factory := agent.NewFactory(llmClient)
	planner := factory.NewPlanner()
	coder := factory.NewCoder()
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

	// ── Workflow Context ─────────────────────────────────────────────────────
	wfCtx := &workflow.Context{
		Requirement: requirement,
	}

	// ── Runner ───────────────────────────────────────────────────────────────
	runner := workflow.NewRunner()
	return runner.Run(context.Background(), planState, wfCtx)
}
