package github

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// RepoInfo はリポジトリのオーナーとリポジトリ名を保持します。
type RepoInfo struct {
	// Owner はリポジトリのオーナー（ユーザー名または組織名）です。
	Owner string
	// Repo はリポジトリ名です。
	Repo string
}

var githubRemotePattern = regexp.MustCompile(`^(?:git@github\.com:|https://github\.com/)([^/]+)/([^/]+?)(?:\.git)?$`)

var issueURLPattern = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/issues/(\d+)$`)

// IssueRef は GitHub Issue URL から抽出したリポジトリ情報と Issue 番号を保持します。
type IssueRef struct {
	// Owner はリポジトリのオーナー名です。
	Owner string
	// Repo はリポジトリ名です。
	Repo string
	// Number は Issue 番号です。
	Number int
}

// ParseIssueURL は GitHub Issue URL（https://github.com/owner/repo/issues/N）を
// パースして IssueRef を返します。URL が対応フォーマットでない場合はエラーを返します。
func ParseIssueURL(rawURL string) (*IssueRef, error) {
	matches := issueURLPattern.FindStringSubmatch(strings.TrimSpace(rawURL))
	if len(matches) != 4 {
		return nil, fmt.Errorf("github: parse issue URL: unsupported issue URL format: %q", rawURL)
	}

	var number int
	if _, err := fmt.Sscan(matches[3], &number); err != nil {
		return nil, fmt.Errorf("github: parse issue URL: invalid issue number %q: %w", matches[3], err)
	}

	return &IssueRef{Owner: matches[1], Repo: matches[2], Number: number}, nil
}

// GetRepoInfo はカレントディレクトリの git origin リモート URL から
// リポジトリの owner と repo を抽出します。
func GetRepoInfo() (owner string, repo string, err error) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", "", fmt.Errorf("github: get repo info: git remote get-url origin: %w", err)
	}

	info, err := parseRemoteURL(strings.TrimSpace(string(out)))
	if err != nil {
		return "", "", fmt.Errorf("github: get repo info: %w", err)
	}

	return info.Owner, info.Repo, nil
}

// DetectRepoInfo はカレントディレクトリの git リモート URL からオーナーとリポジトリ名を抽出します。
// SSH 形式（git@github.com:owner/repo.git）および HTTPS 形式（https://github.com/owner/repo.git）に対応します。
func DetectRepoInfo(ctx context.Context) (*RepoInfo, error) {
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("github: detect repo info: %w", err)
	}
	return parseRemoteURL(strings.TrimSpace(string(out)))
}

// DetectBaseBranch はカレントディレクトリのブランチ名を返します。
func DetectBaseBranch(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("github: detect base branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// parseRemoteURL はリモート URL をパースして RepoInfo を返します。
// SSH 形式と HTTPS 形式の両方に対応します。
func parseRemoteURL(rawURL string) (*RepoInfo, error) {
	matches := githubRemotePattern.FindStringSubmatch(strings.TrimSpace(rawURL))
	if len(matches) != 3 {
		return nil, fmt.Errorf("github: parse remote URL: unsupported remote URL format: %q", rawURL)
	}

	return &RepoInfo{Owner: matches[1], Repo: matches[2]}, nil
}
