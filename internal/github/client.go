// Package github は GitHub API と通信するクライアントを提供します。
// Client インターフェースを中心に、GitHubClient（実装）と MockClient（テスト用）を定義します。
package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gogithub "github.com/google/go-github/v72/github"
)

// Issue は GitHub Issue のドメイン構造体です。
type Issue struct {
	// Number は Issue 番号です。
	Number int
	// Title は Issue のタイトルです。
	Title string
	// Body は Issue の本文です。
	Body string
}

// PullRequest は GitHub Pull Request のドメイン構造体です。
type PullRequest struct {
	// Number は PR 番号です。
	Number int
	// URL は PR の HTML URL です。
	URL string
}

// Client は GitHub API と通信するインターフェースです。
// 実装はゴルーチン安全である必要があります。
type Client interface {
	// GetIssue は指定された番号の Issue を取得します。
	GetIssue(ctx context.Context, owner, repo string, number int) (*Issue, error)
	// CreateIssue は新しい Issue を作成し、採番された Issue を返します。
	CreateIssue(ctx context.Context, owner, repo string, title, body string) (*Issue, error)
	// CreatePullRequest は新しい Pull Request を作成します。
	// head ブランチに対してすでに PR が存在する場合はエラーを返します。
	CreatePullRequest(ctx context.Context, owner, repo string, title, head, base, body string) (*PullRequest, error)
}

// GitHubClient は Client インターフェースの実装です。
// GITHUB_TOKEN による認証付きの *gogithub.Client を内部に保持します。
type GitHubClient struct {
	client *gogithub.Client
}

// NewGitHubClient は token を使用して GitHubClient を初期化します。
// token は GitHub の Personal Access Token または Actions の GITHUB_TOKEN を指定します。
func NewGitHubClient(token string) *GitHubClient {
	return &GitHubClient{
		client: gogithub.NewClient(nil).WithAuthToken(token),
	}
}

// GetIssue は指定された番号の Issue を取得します。
func (c *GitHubClient) GetIssue(ctx context.Context, owner, repo string, number int) (*Issue, error) {
	issue, _, err := c.client.Issues.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("github: get issue #%d: %w", number, err)
	}
	return &Issue{
		Number: issue.GetNumber(),
		Title:  issue.GetTitle(),
		Body:   issue.GetBody(),
	}, nil
}

// CreateIssue は新しい Issue を作成し、採番された Issue を返します。
func (c *GitHubClient) CreateIssue(ctx context.Context, owner, repo string, title, body string) (*Issue, error) {
	req := &gogithub.IssueRequest{
		Title: gogithub.Ptr(title),
		Body:  gogithub.Ptr(body),
	}
	issue, _, err := c.client.Issues.Create(ctx, owner, repo, req)
	if err != nil {
		return nil, fmt.Errorf("github: create issue: %w", err)
	}
	return &Issue{
		Number: issue.GetNumber(),
		Title:  issue.GetTitle(),
		Body:   issue.GetBody(),
	}, nil
}

// CreatePullRequest は新しい Pull Request を作成します。
// head ブランチに対してすでに PR が存在する場合（HTTP 422）は、
// エラーメッセージに head ブランチ名を含めてラップして返します。
func (c *GitHubClient) CreatePullRequest(ctx context.Context, owner, repo string, title, head, base, body string) (*PullRequest, error) {
	req := &gogithub.NewPullRequest{
		Title: gogithub.Ptr(title),
		Head:  gogithub.Ptr(head),
		Base:  gogithub.Ptr(base),
		Body:  gogithub.Ptr(body),
	}
	pr, _, err := c.client.PullRequests.Create(ctx, owner, repo, req)
	if err != nil {
		var errResp *gogithub.ErrorResponse
		if errors.As(err, &errResp) &&
			errResp.Response != nil &&
			errResp.Response.StatusCode == http.StatusUnprocessableEntity {
			return nil, fmt.Errorf("github: create pull request: head %q already has an open PR: %w", head, err)
		}
		return nil, fmt.Errorf("github: create pull request: %w", err)
	}
	return &PullRequest{
		Number: pr.GetNumber(),
		URL:    pr.GetHTMLURL(),
	}, nil
}
