package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/takiguchi-yu/cording-pilot/internal/agent"
	"github.com/takiguchi-yu/cording-pilot/internal/tui"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

// InteractiveState は ⓪ 対話フェーズです。
// Planner Agent が要件の不足を分析して質問リストを生成し、TUI フォームでユーザーへ提示します。
// ユーザーの回答を元に最終的な実装計画（擬似 Issue）を生成し、wfCtx に保存して Next へ遷移します。
type InteractiveState struct {
	// Planner は要件の分析と Issue のコンパイルを担当する計画エージェントです。
	Planner agent.PlannerAgent
	// Logger は処理ログを出力する NDJSON ロガーです。
	Logger *logger.Logger
	// Next は後継ステート（通常は PlanState）です。
	Next State
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

	return s.Next, nil
}
