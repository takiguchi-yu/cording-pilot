package github_test

import (
	"strings"
	"testing"

	githubpkg "github.com/takiguchi-yu/cording-pilot/internal/github"
)

func TestParseRemoteURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rawURL    string
		wantOwner string
		wantRepo  string
		wantErr   string
	}{
		{
			name:      "SSH .git あり",
			rawURL:    "git@github.com:takiguchi-yu/cording-pilot.git",
			wantOwner: "takiguchi-yu",
			wantRepo:  "cording-pilot",
		},
		{
			name:      "SSH .git なし",
			rawURL:    "git@github.com:takiguchi-yu/cording-pilot",
			wantOwner: "takiguchi-yu",
			wantRepo:  "cording-pilot",
		},
		{
			name:      "HTTPS .git あり",
			rawURL:    "https://github.com/takiguchi-yu/cording-pilot.git",
			wantOwner: "takiguchi-yu",
			wantRepo:  "cording-pilot",
		},
		{
			name:      "HTTPS .git なし",
			rawURL:    "https://github.com/takiguchi-yu/cording-pilot",
			wantOwner: "takiguchi-yu",
			wantRepo:  "cording-pilot",
		},
		{
			name:    "GitHub 以外はエラー",
			rawURL:  "https://example.com/takiguchi-yu/cording-pilot.git",
			wantErr: "unsupported remote URL format",
		},
		{
			name:    "owner/repo 不足はエラー",
			rawURL:  "git@github.com:takiguchi-yu",
			wantErr: "unsupported remote URL format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := githubpkg.ExportParseRemoteURL(tt.rawURL)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Owner != tt.wantOwner {
				t.Errorf("Owner = %q, want %q", got.Owner, tt.wantOwner)
			}
			if got.Repo != tt.wantRepo {
				t.Errorf("Repo = %q, want %q", got.Repo, tt.wantRepo)
			}
		})
	}
}

func TestParseIssueURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rawURL    string
		wantOwner string
		wantRepo  string
		wantNum   int
		wantErr   string
	}{
		{
			name:      "標準形式",
			rawURL:    "https://github.com/takiguchi-yu/cording-pilot/issues/42",
			wantOwner: "takiguchi-yu",
			wantRepo:  "cording-pilot",
			wantNum:   42,
		},
		{
			name:      "Issue 番号 1 桁",
			rawURL:    "https://github.com/org/repo/issues/1",
			wantOwner: "org",
			wantRepo:  "repo",
			wantNum:   1,
		},
		{
			name:    "PR URL はエラー",
			rawURL:  "https://github.com/org/repo/pull/10",
			wantErr: "unsupported issue URL format",
		},
		{
			name:    "GitHub 以外はエラー",
			rawURL:  "https://example.com/org/repo/issues/1",
			wantErr: "unsupported issue URL format",
		},
		{
			name:    "番号なしはエラー",
			rawURL:  "https://github.com/org/repo/issues/",
			wantErr: "unsupported issue URL format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := githubpkg.ExportParseIssueURL(tt.rawURL)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Owner != tt.wantOwner {
				t.Errorf("Owner = %q, want %q", got.Owner, tt.wantOwner)
			}
			if got.Repo != tt.wantRepo {
				t.Errorf("Repo = %q, want %q", got.Repo, tt.wantRepo)
			}
			if got.Number != tt.wantNum {
				t.Errorf("Number = %d, want %d", got.Number, tt.wantNum)
			}
		})
	}
}
