package workflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	githubpkg "github.com/takiguchi-yu/cording-pilot/internal/github"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

type stubLLMClient struct {
	generateFn func(ctx context.Context, prompt string) (string, error)
}

func (s *stubLLMClient) Generate(ctx context.Context, prompt string) (string, error) {
	if s.generateFn == nil {
		return "", nil
	}
	return s.generateFn(ctx, prompt)
}

func (s *stubLLMClient) GenerateStructured(_ context.Context, _ string, _ interface{}) error {
	return nil
}

func newTestLogger() *logger.Logger {
	return logger.New(&strings.Builder{})
}

func TestCompleteState_generatePRBody_正常系でフェンス除去しClosesを追記する(t *testing.T) {
	t.Parallel()

	cloneDir := t.TempDir()
	template := "## 背景\n- TODO\n\n## やったこと\n- TODO\n"
	if err := os.MkdirAll(filepath.Join(cloneDir, ".github"), 0o755); err != nil {
		t.Fatalf("failed to mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cloneDir, ".github", "pull_request_template.md"), []byte(template), 0o600); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	s := &CompleteState{
		Logger: newTestLogger(),
		LLMClient: &stubLLMClient{generateFn: func(_ context.Context, prompt string) (string, error) {
			if !strings.Contains(prompt, "【実装計画】") {
				t.Fatalf("prompt should contain plan section")
			}
			if !strings.Contains(prompt, "【PRテンプレート】") {
				t.Fatalf("prompt should contain template section")
			}
			return "```markdown\n## 背景\n- 変更背景\n\n## やったこと\n- 実装内容\n```", nil
		}},
	}

	body, err := s.generatePRBody(context.Background(), &Context{PlanText: "計画A", IssueNumber: 42}, cloneDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(body, "```") {
		t.Fatalf("code fence should be sanitized: %q", body)
	}
	if !strings.Contains(body, "## 背景") {
		t.Fatalf("generated body should contain heading")
	}
	if !strings.Contains(body, "Closes #42") {
		t.Fatalf("generated body should contain issue close keyword")
	}
}

func TestCompleteState_generatePRBody_LLMエラー時にフォールバックを返す(t *testing.T) {
	t.Parallel()

	cloneDir := t.TempDir()
	template := "## 背景\n- TODO\n"
	if err := os.MkdirAll(filepath.Join(cloneDir, ".github"), 0o755); err != nil {
		t.Fatalf("failed to mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cloneDir, ".github", "pull_request_template.md"), []byte(template), 0o600); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	s := &CompleteState{
		Logger: newTestLogger(),
		LLMClient: &stubLLMClient{generateFn: func(_ context.Context, _ string) (string, error) {
			return "", errors.New("llm unavailable")
		}},
	}

	body, err := s.generatePRBody(context.Background(), &Context{PlanText: "実装計画の要約", IssueNumber: 7}, cloneDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(body, "## 背景") {
		t.Fatalf("fallback should include template: %q", body)
	}
	if !strings.Contains(body, "## 実装計画") {
		t.Fatalf("fallback should include plan section: %q", body)
	}
	if !strings.Contains(body, "Closes #7") {
		t.Fatalf("fallback should include issue close keyword")
	}
}

func TestCompleteState_generatePRBody_Issue情報をプロンプトへ含める(t *testing.T) {
	t.Parallel()

	cloneDir := t.TempDir()
	var capturedPrompt string

	s := &CompleteState{
		Logger: newTestLogger(),
		GitHub: &githubpkg.MockClient{
			GetIssueFunc: func(_ context.Context, _ string, _ string, number int) (*githubpkg.Issue, error) {
				return &githubpkg.Issue{Number: number, Title: "Issue title", Body: "Issue body"}, nil
			},
			CreateIssueFunc: func(_ context.Context, _ string, _ string, _ string, _ string) (*githubpkg.Issue, error) {
				return nil, nil
			},
			CreatePullRequestFunc: func(_ context.Context, _ string, _ string, _ string, _ string, _ string, _ string) (*githubpkg.PullRequest, error) {
				return nil, nil
			},
		},
		RepoOwner: "owner",
		RepoName:  "repo",
		LLMClient: &stubLLMClient{generateFn: func(_ context.Context, prompt string) (string, error) {
			capturedPrompt = prompt
			return "本文", nil
		}},
	}

	_, err := s.generatePRBody(context.Background(), &Context{PlanText: "plan", IssueNumber: 3}, cloneDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(capturedPrompt, "【Issue情報】") {
		t.Fatalf("prompt should contain issue section")
	}
	if !strings.Contains(capturedPrompt, "Issue title") {
		t.Fatalf("prompt should include issue title")
	}
}
