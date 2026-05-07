package workflow

import (
	"context"
	"fmt"

	"github.com/takiguchi-yu/cording-pilot/internal/agent"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

// PlanState は ① 計画フェーズです。
// wfCtx に保存された要件を元に Planner エージェントに実装計画を生成させ、Next へ遷移します。
type PlanState struct {
	Planner agent.Agent
	Logger  *logger.Logger
	// Next は後継ステート（通常は ImplementState）です。
	Next State
}

// Execute implements State.
func (s *PlanState) Execute(ctx context.Context, wfCtx *Context) (State, error) {
	if err := s.Logger.Info("plan.start", "① 計画フェーズを開始します"); err != nil {
		return nil, err
	}

	plan, err := s.Planner.Ask(ctx, fmt.Sprintf("[PLAN] 以下の要件について実装計画を作成してください。\n\n%s", wfCtx.Requirement))
	if err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}

	wfCtx.PlanText = plan

	if err = s.Logger.Info("plan.done", plan); err != nil {
		return nil, err
	}

	return s.Next, nil
}
