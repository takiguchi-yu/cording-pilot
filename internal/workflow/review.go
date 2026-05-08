package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/takiguchi-yu/cording-pilot/internal/agent"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

const (
	maxReviewRequirementChars = 2000
	maxReviewPlanChars        = 2500
	maxReviewTestOutputChars  = 3000
	reviewLLMTimeout          = 2 * time.Minute
)

// ReviewState は ③ レビューフェーズです。
// Reviewer エージェントが実装を元の要件と照合し、承認または修正要求のどちらかを返します。
// 承認時は OnApprove へ、却下時は OnReject（通常は ImplementState）へ遷移します。
type ReviewState struct {
	Reviewer agent.Agent
	Logger   *logger.Logger
	// OnApprove はレビュー承認時の後継ステート（通常は CompleteState）です。
	OnApprove State
	// OnReject は修正要求時の後継ステート（通常は ImplementState）です。
	OnReject State
}

// Execute implements State.
func (s *ReviewState) Execute(ctx context.Context, wfCtx *Context) (State, error) {
	if err := s.Logger.Info("review.start", "③ レビューフェーズを開始します"); err != nil {
		return nil, err
	}

	if wfCtx.DeterministicFallbackUsed {
		wfCtx.ReviewFeedback = "Approve: deterministic fallback を適用し、品質パイプラインを通過済みのためレビューをスキップしました。"
		if err := s.Logger.Warn("review.skipped", "deterministic fallback 適用済みのため Reviewer 呼び出しをスキップします"); err != nil {
			return nil, err
		}
		return s.OnApprove, nil
	}

	requirementForPrompt := compactPromptText(wfCtx.Requirement, maxReviewRequirementChars)
	planForPrompt := compactPromptText(wfCtx.PlanText, maxReviewPlanChars)
	testOutputForPrompt := compactPromptText(wfCtx.LastTestOutput, maxReviewTestOutputChars)
	prompt := fmt.Sprintf(
		"[REVIEW] 以下の要件と実装計画、テスト結果を元にコードレビューを行ってください。\n\n## 要件\n%s\n\n## 実装計画\n%s\n\n## テスト結果\n%s",
		requirementForPrompt,
		planForPrompt,
		testOutputForPrompt,
	)

	reviewCtx, cancel := context.WithTimeout(ctx, reviewLLMTimeout)
	defer cancel()

	feedback, err := s.Reviewer.Ask(reviewCtx, prompt)
	if err != nil {
		return nil, fmt.Errorf("review: %w", err)
	}
	wfCtx.ReviewFeedback = feedback

	if err = s.Logger.Info("review.result", feedback); err != nil {
		return nil, err
	}

	if isApprovedFeedback(feedback) {
		if err = s.Logger.Info("review.approved", "レビュー承認 → 完了フェーズへ移行します"); err != nil {
			return nil, err
		}
		return s.OnApprove, nil
	}

	if err = s.Logger.Warn("review.rejected", "変更が要求されました → 実装フェーズへ差し戻します"); err != nil {
		return nil, err
	}

	// Reset the fix-loop counter for the next implementation cycle.
	wfCtx.TryCount = 0
	wfCtx.PlanText = fmt.Sprintf("%s\n\n## レビューフィードバック\n%s", wfCtx.PlanText, feedback)

	return s.OnReject, nil
}

func isApprovedFeedback(feedback string) bool {
	for _, line := range strings.Split(feedback, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		parts := strings.Fields(trimmed)
		if len(parts) == 0 {
			continue
		}
		first := strings.Trim(parts[0], ":.-")
		return strings.EqualFold(first, "approve")
	}
	return false
}
