package github

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// RepoInfo はリポジトリのオーナーとリポジトリ名を保持します。
type RepoInfo struct {
	// Owner はリポジトリのオーナー（ユーザー名または組織名）です。
	Owner string
	// Repo はリポジトリ名です。
	Repo string
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
	url := strings.TrimSuffix(rawURL, ".git")

	// SSH 形式: git@github.com:owner/repo
	if strings.HasPrefix(url, "git@") {
		parts := strings.SplitN(url, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("github: parse remote URL: unexpected SSH URL format: %q", rawURL)
		}
		ownerRepo := strings.SplitN(parts[1], "/", 2)
		if len(ownerRepo) != 2 {
			return nil, fmt.Errorf("github: parse remote URL: unexpected path in SSH URL: %q", parts[1])
		}
		return &RepoInfo{Owner: ownerRepo[0], Repo: ownerRepo[1]}, nil
	}

	// HTTPS 形式: https://github.com/owner/repo
	parts := strings.Split(url, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("github: parse remote URL: unexpected HTTPS URL format: %q", rawURL)
	}
	return &RepoInfo{
		Owner: parts[len(parts)-2],
		Repo:  parts[len(parts)-1],
	}, nil
}
