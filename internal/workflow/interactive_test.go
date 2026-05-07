package workflow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/takiguchi-yu/cording-pilot/internal/agent"
	githubpkg "github.com/takiguchi-yu/cording-pilot/internal/github"
	"github.com/takiguchi-yu/cording-pilot/internal/tui"
	"github.com/takiguchi-yu/cording-pilot/internal/workflow"
)

// stubPlannerAgent は InteractiveState のテスト用 PlannerAgent スタブです。
type stubPlannerAgent struct {
	clarification agent.ClarificationRequest
	clarifyErr    error
	compiledIssue string
	compileErr    error
}

func (s *stubPlannerAgent) Ask(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (s *stubPlannerAgent) GenerateClarification(_ context.Context, _ string) (agent.ClarificationRequest, error) {
	return s.clarification, s.clarifyErr
}

func (s *stubPlannerAgent) CompileIssue(_ context.Context, _ string, _ map[string]string, _ string) (string, error) {
	return s.compiledIssue, s.compileErr
}

func TestInteractiveState_要件が明確な場合スキップ(t *testing.T) {
	t.Parallel()

	nextState := &workflow.PlanState{
		Planner: &stubPlannerAgent{},
		Logger:  newLogger(),
	}
	stub := &stubPlannerAgent{
		clarification: agent.ClarificationRequest{IsClear: true},
		compiledIssue: "# タイトル\n\n本文",
	}
	s := &workflow.InteractiveState{
		Planner: stub,
		Logger:  newLogger(),
		Next:    nextState,
		SelectIssueType: func() (string, error) {
			return "feature", nil
		},
		LoadIssueTemplate: func(string) (string, error) {
			return "", nil
		},
	}

	wfCtx := &workflow.Context{Requirement: "明確な要件"}
	next, err := s.Execute(context.Background(), wfCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next != nextState {
		t.Error("expected nextState to be returned")
	}
	if wfCtx.Requirement != "# タイトル\n\n本文" {
		t.Errorf("unexpected compiled requirement: %q", wfCtx.Requirement)
	}
}

func TestInteractiveState_質問が空の場合スキップ(t *testing.T) {
	t.Parallel()

	nextState := &workflow.PlanState{
		Planner: &stubPlannerAgent{},
		Logger:  newLogger(),
	}
	stub := &stubPlannerAgent{
		clarification: agent.ClarificationRequest{IsClear: false, Questions: nil},
		compiledIssue: "# タイトル\n\n本文",
	}
	s := &workflow.InteractiveState{
		Planner: stub,
		Logger:  newLogger(),
		Next:    nextState,
		SelectIssueType: func() (string, error) {
			return "feature", nil
		},
		LoadIssueTemplate: func(string) (string, error) {
			return "", nil
		},
	}

	wfCtx := &workflow.Context{Requirement: "要件"}
	next, err := s.Execute(context.Background(), wfCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next != nextState {
		t.Error("expected nextState to be returned")
	}
}

func TestInteractiveState_GenerateClarification失敗時にエラーを返す(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("clarify error")
	stub := &stubPlannerAgent{clarifyErr: wantErr}
	s := &workflow.InteractiveState{
		Planner: stub,
		Logger:  newLogger(),
		SelectIssueType: func() (string, error) {
			return "feature", nil
		},
		LoadIssueTemplate: func(string) (string, error) {
			return "", nil
		},
	}

	_, err := s.Execute(context.Background(), &workflow.Context{Requirement: "要件"})
	if !errors.Is(err, wantErr) {
		t.Errorf("expected wrapped error %v; got %v", wantErr, err)
	}
}

func TestInteractiveState_ユーザー中断時にErrAbortedを返す(t *testing.T) {
	t.Parallel()

	s := &workflow.InteractiveState{
		Planner: &stubPlannerAgent{},
		Logger:  newLogger(),
		SelectIssueType: func() (string, error) {
			return "", tui.ErrAborted
		},
	}

	_, err := s.Execute(context.Background(), &workflow.Context{Requirement: "要件"})
	if !errors.Is(err, tui.ErrAborted) {
		t.Errorf("expected wrapped error %v; got %v", tui.ErrAborted, err)
	}
}

func TestInteractiveState_Issue作成時にH1がない場合は本文からタイトルを推定する(t *testing.T) {
	t.Parallel()

	const compiled = "## 概要\n\nIssue 作成時に適切なタイトルが設定されるようにする。\n\n## 受け入れ条件\n\n- 先頭のセクション見出しはタイトルにしない"

	stub := &stubPlannerAgent{
		clarification: agent.ClarificationRequest{IsClear: true},
		compiledIssue: compiled,
	}

	var gotTitle string
	var gotBody string

	s := &workflow.InteractiveState{
		Planner: stub,
		Logger:  newLogger(),
		Next:    &workflow.PlanState{Planner: &stubPlannerAgent{}, Logger: newLogger()},
		GitHub: &githubpkg.MockClient{
			GetIssueFunc: func(context.Context, string, string, int) (*githubpkg.Issue, error) {
				return nil, nil
			},
			CreateIssueFunc: func(_ context.Context, _, _, title, body string) (*githubpkg.Issue, error) {
				gotTitle = title
				gotBody = body
				return &githubpkg.Issue{Number: 10, Title: title, Body: body}, nil
			},
			CreatePullRequestFunc: func(context.Context, string, string, string, string, string, string) (*githubpkg.PullRequest, error) {
				return nil, nil
			},
		},
		RepoOwner: "owner",
		RepoName:  "repo",
		SelectIssueType: func() (string, error) {
			return "feature", nil
		},
		LoadIssueTemplate: func(string) (string, error) {
			return "", nil
		},
	}

	wfCtx := &workflow.Context{Requirement: "要件"}
	if _, err := s.Execute(context.Background(), wfCtx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotTitle != "Issue 作成時に適切なタイトルが設定されるようにする。" {
		t.Errorf("unexpected title: %q", gotTitle)
	}
	if gotBody != compiled {
		t.Errorf("unexpected body: %q", gotBody)
	}
}

func TestInteractiveState_Issue作成時に先頭H1をタイトルとして使う(t *testing.T) {
	t.Parallel()

	const compiled = "# Issueタイトル\n\n## 概要\n\n本文"

	stub := &stubPlannerAgent{
		clarification: agent.ClarificationRequest{IsClear: true},
		compiledIssue: compiled,
	}

	var gotTitle string
	var gotBody string

	s := &workflow.InteractiveState{
		Planner: stub,
		Logger:  newLogger(),
		Next:    &workflow.PlanState{Planner: &stubPlannerAgent{}, Logger: newLogger()},
		GitHub: &githubpkg.MockClient{
			GetIssueFunc: func(context.Context, string, string, int) (*githubpkg.Issue, error) {
				return nil, nil
			},
			CreateIssueFunc: func(_ context.Context, _, _, title, body string) (*githubpkg.Issue, error) {
				gotTitle = title
				gotBody = body
				return &githubpkg.Issue{Number: 11, Title: title, Body: body}, nil
			},
			CreatePullRequestFunc: func(context.Context, string, string, string, string, string, string) (*githubpkg.PullRequest, error) {
				return nil, nil
			},
		},
		RepoOwner: "owner",
		RepoName:  "repo",
		SelectIssueType: func() (string, error) {
			return "feature", nil
		},
		LoadIssueTemplate: func(string) (string, error) {
			return "", nil
		},
	}

	wfCtx := &workflow.Context{Requirement: "要件"}
	if _, err := s.Execute(context.Background(), wfCtx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotTitle != "Issueタイトル" {
		t.Errorf("unexpected title: %q", gotTitle)
	}
	if gotBody != "## 概要\n\n本文" {
		t.Errorf("unexpected body: %q", gotBody)
	}
}
