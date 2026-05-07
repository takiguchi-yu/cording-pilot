package github

import "context"

// MockClient はテスト用の Client 実装です。
// 各メソッドに対応する関数フィールドを設定することでふるまいをカスタマイズできます。
type MockClient struct {
	// GetIssueFunc は GetIssue 呼び出しを処理する関数です。
	GetIssueFunc func(ctx context.Context, owner, repo string, number int) (*Issue, error)
	// CreateIssueFunc は CreateIssue 呼び出しを処理する関数です。
	CreateIssueFunc func(ctx context.Context, owner, repo string, title, body string) (*Issue, error)
	// CreatePullRequestFunc は CreatePullRequest 呼び出しを処理する関数です。
	CreatePullRequestFunc func(ctx context.Context, owner, repo string, title, head, base, body string) (*PullRequest, error)
}

// GetIssue は MockClient.GetIssueFunc を委譲します。
func (m *MockClient) GetIssue(ctx context.Context, owner, repo string, number int) (*Issue, error) {
	return m.GetIssueFunc(ctx, owner, repo, number)
}

// CreateIssue は MockClient.CreateIssueFunc を委譲します。
func (m *MockClient) CreateIssue(ctx context.Context, owner, repo string, title, body string) (*Issue, error) {
	return m.CreateIssueFunc(ctx, owner, repo, title, body)
}

// CreatePullRequest は MockClient.CreatePullRequestFunc を委譲します。
func (m *MockClient) CreatePullRequest(ctx context.Context, owner, repo string, title, head, base, body string) (*PullRequest, error) {
	return m.CreatePullRequestFunc(ctx, owner, repo, title, head, base, body)
}
