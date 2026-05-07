package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/takiguchi-yu/cording-pilot/internal/agent"
	githubpkg "github.com/takiguchi-yu/cording-pilot/internal/github"
	"github.com/takiguchi-yu/cording-pilot/internal/tui"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

// InteractiveState は ⓪ 対話フェーズです。
// Planner Agent が要件の不足を分析して質問リストを生成し、TUI フォームでユーザーへ提示します。
// ユーザーの回答を元に最終的な実装計画（擬似 Issue）を生成し、wfCtx に保存して Next へ遷移します。
//
// GitHub が設定されている場合:
//   - wfCtx.IssueNumber > 0 なら GetIssue で本文を取得して Requirement に設定する。
//   - 新規要件の場合は CompileIssue 後に CreateIssue を実行し、IssueNumber を採番する。
type InteractiveState struct {
	// Planner は要件の分析と Issue のコンパイルを担当する計画エージェントです。
	Planner agent.PlannerAgent
	// Logger は処理ログを出力する NDJSON ロガーです。
	Logger *logger.Logger
	// Next は後継ステート（通常は PlanState）です。
	Next State
	// GitHub は GitHub API クライアントです。nil の場合は GitHub 連携をスキップします。
	GitHub githubpkg.Client
	// RepoOwner はリポジトリのオーナー名です。
	RepoOwner string
	// RepoName はリポジトリ名です。
	RepoName string
}

// Execute implements State.
//
// LLM との通信には ctx（タイムアウトあり）を使用し、
// TUI フォームの入力待機には context.Background()（タイムアウトなし）を使用します。
// これにより、ユーザーの入力中に LLM タイムアウトが発火してフォームが中断されることを防ぎます。
func (s *InteractiveState) Execute(ctx context.Context, wfCtx *Context) (State, error) {
	if err := s.Logger.Info("interactive.start", "⓪ 対話フェーズを開始します"); err != nil {
		return nil, err
	}

	// ── Step 0: Issue 番号が指定されている場合は GitHub から本文を取得 ──────────────
	if wfCtx.IssueNumber > 0 && s.GitHub != nil {
		return s.fetchIssue(ctx, wfCtx)
	}

	// ── Step 1: 要件の不足を分析して質問リストを生成 ────────────────────────────
	clarification, err := s.Planner.GenerateClarification(ctx, wfCtx.Requirement)
	if err != nil {
		return nil, fmt.Errorf("interactive: generate clarification: %w", err)
	}

	// ── Step 2: 要件が十分明確な場合は TUI をスキップ ────────────────────────────
	if clarification.IsClear || len(clarification.Questions) == 0 {
		if err = s.Logger.Info("interactive.skip", "要件が十分明確なため、ヒアリングをスキップします"); err != nil {
			return nil, err
		}
		if err = s.maybeCreateIssue(ctx, wfCtx); err != nil {
			return nil, err
		}
		return s.Next, nil
	}

	if err = s.Logger.Info("interactive.questions", fmt.Sprintf("%d 件の確認事項を生成しました", len(clarification.Questions))); err != nil {
		return nil, err
	}

	// ── Step 3: TUI フォームでユーザーへヒアリング ─────────────────────────────
	// TUI の入力待機は context.Background() で行い、LLM タイムアウトの影響を受けないようにします。
	answers, err := tui.RunForm(clarification.Questions)
	if err != nil {
		if errors.Is(err, tui.ErrAborted) {
			return nil, fmt.Errorf("interactive: %w", tui.ErrAborted)
		}
		return nil, fmt.Errorf("interactive: form: %w", err)
	}

	if err = s.Logger.Info("interactive.answers_collected", fmt.Sprintf("%d 件の回答を収集しました", len(answers))); err != nil {
		return nil, err
	}

	// ── Step 4: 回答を元に最終的な実装計画（Issue）をコンパイル ─────────────────
	compiled, err := s.Planner.CompileIssue(ctx, wfCtx.Requirement, answers)
	if err != nil {
		return nil, fmt.Errorf("interactive: compile issue: %w", err)
	}

	wfCtx.Requirement = compiled

	if err = s.Logger.Info("interactive.done", compiled); err != nil {
		return nil, err
	}

	// ── Step 5: GitHub Issue を作成 ──────────────────────────────────────────
	if err = s.maybeCreateIssue(ctx, wfCtx); err != nil {
		return nil, err
	}

	return s.Next, nil
}

// fetchIssue は GitHub から Issue を取得し、本文を Requirement に設定します。
func (s *InteractiveState) fetchIssue(ctx context.Context, wfCtx *Context) (State, error) {
	if err := s.Logger.Info(
		"interactive.fetch_issue",
		fmt.Sprintf("Issue #%d を GitHub から取得します", wfCtx.IssueNumber),
	); err != nil {
		return nil, err
	}

	issue, err := s.GitHub.GetIssue(ctx, s.RepoOwner, s.RepoName, wfCtx.IssueNumber)
	if err != nil {
		return nil, fmt.Errorf("interactive: fetch issue #%d: %w", wfCtx.IssueNumber, err)
	}

	wfCtx.Requirement = fmt.Sprintf("# %s\n\n%s", issue.Title, issue.Body)

	if err = s.Logger.Info(
		"interactive.issue_fetched",
		fmt.Sprintf("Issue #%d を取得しました: %s", issue.Number, issue.Title),
	); err != nil {
		return nil, err
	}

	return s.Next, nil
}

// maybeCreateIssue は GitHub クライアントが設定されている場合に Issue を作成します。
// エラーが発生した場合は警告ログを出力しますが、ワークフローは継続します。
func (s *InteractiveState) maybeCreateIssue(ctx context.Context, wfCtx *Context) error {
	if s.GitHub == nil || wfCtx.IssueNumber > 0 {
		return nil
	}

	title, body := splitIssueText(wfCtx.Requirement)
	issue, err := s.GitHub.CreateIssue(ctx, s.RepoOwner, s.RepoName, title, body)
	if err != nil {
		// Issue 作成失敗はワークフローを停止しない（警告のみ）。
		_ = s.Logger.Info("interactive.issue_create_warn", fmt.Sprintf("Issue の作成に失敗しました（スキップ）: %v", err))
		return nil
	}

	wfCtx.IssueNumber = issue.Number
	return s.Logger.Info(
		"interactive.issue_created",
		fmt.Sprintf("Issue #%d を作成しました: %s", issue.Number, issue.Title),
	)
}

// splitIssueText は Markdown テキストの先頭行をタイトル、残りを本文として分割します。
// 先頭行の Markdown 見出し記号（#）は除去されます。
func splitIssueText(text string) (title, body string) {
	lines := strings.SplitN(text, "\n", 2)
	title = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(lines[0]), "#"))
	if len(lines) > 1 {
		body = strings.TrimSpace(lines[1])
	}
	return
}
