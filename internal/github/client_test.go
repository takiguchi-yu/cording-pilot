package github_test

import (
	"context"
	"errors"
	"testing"

	githubpkg "github.com/takiguchi-yu/cording-pilot/internal/github"
)

func TestMockClient_GetIssue(t *testing.T) {
	t.Parallel()

	want := &githubpkg.Issue{Number: 42, Title: "テスト Issue", Body: "テスト本文"}
	mock := &githubpkg.MockClient{
		GetIssueFunc: func(_ context.Context, _, _ string, _ int) (*githubpkg.Issue, error) {
			return want, nil
		},
	}

	got, err := mock.GetIssue(context.Background(), "owner", "repo", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Number != want.Number || got.Title != want.Title {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestMockClient_CreateIssue(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("create issue error")
	mock := &githubpkg.MockClient{
		CreateIssueFunc: func(_ context.Context, _, _, _, _ string) (*githubpkg.Issue, error) {
			return nil, wantErr
		},
	}

	_, err := mock.CreateIssue(context.Background(), "owner", "repo", "タイトル", "本文")
	if !errors.Is(err, wantErr) {
		t.Errorf("want error %v, got %v", wantErr, err)
	}
}

func TestMockClient_CreatePullRequest(t *testing.T) {
	t.Parallel()

	want := &githubpkg.PullRequest{Number: 1, URL: "https://github.com/owner/repo/pull/1"}
	mock := &githubpkg.MockClient{
		CreatePullRequestFunc: func(_ context.Context, _, _, _, _, _, _ string) (*githubpkg.PullRequest, error) {
			return want, nil
		},
	}

	got, err := mock.CreatePullRequest(context.Background(), "owner", "repo", "title", "head", "main", "body")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Number != want.Number || got.URL != want.URL {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
