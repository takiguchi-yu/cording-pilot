package workflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/takiguchi-yu/cording-pilot/internal/agent"
	"github.com/takiguchi-yu/cording-pilot/internal/workflow"
)

func TestTruncateLog(t *testing.T) {
	t.Parallel()

	makeLines := func(n int, prefix string) string {
		var sb strings.Builder
		for i := 0; i < n; i++ {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(prefix)
		}
		return sb.String()
	}

	tests := []struct {
		name       string
		output     string
		maxLines   int
		wantSnip   bool
		wantPrefix string
	}{
		{
			name:     "行数が maxLines 以内なら変更しない",
			output:   makeLines(30, "line"),
			maxLines: 50,
			wantSnip: false,
		},
		{
			name:     "行数が maxLines ちょうどなら変更しない",
			output:   makeLines(50, "line"),
			maxLines: 50,
			wantSnip: false,
		},
		{
			name:     "行数が maxLines を超えたら snip マーカーを挿入する",
			output:   makeLines(100, "failure-line"),
			maxLines: 50,
			wantSnip: true,
		},
		{
			name:     "先頭10行は必ず保持される",
			output:   makeLines(200, "important"),
			maxLines: 50,
			wantSnip: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := workflow.TruncateLog(tc.output, tc.maxLines)
			hasSnip := strings.Contains(got, "(snip")

			if tc.wantSnip && !hasSnip {
				t.Errorf("snip マーカーが期待されましたが含まれていません: %q", got)
			}
			if !tc.wantSnip && hasSnip {
				t.Errorf("snip マーカーは含まれないはずですが含まれています: %q", got)
			}

			if tc.wantSnip {
				gotLines := strings.Split(got, "\n")
				// snip マーカー行を1行と数えると、合計行数は maxLines+1 以内
				if len(gotLines) > tc.maxLines+2 {
					t.Errorf("切り詰め後の行数 %d が maxLines %d を大幅に超えています", len(gotLines), tc.maxLines)
				}
			}
		})
	}
}

func TestFilterIssueForCoder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		issue     string
		wantParts []string
		noParts   []string
		wantFull  bool
	}{
		{
			name: "必要セクションのみを保持する",
			issue: strings.Join([]string{
				"# タイトル",
				"",
				"## 概要",
				"背景説明",
				"",
				"## 要件・仕様",
				"- API を追加する",
				"",
				"## 調査方針",
				"- 調査項目",
				"",
				"## 受け入れ条件",
				"- テストが通る",
				"",
				"## 制約事項・影響範囲",
				"- 既存 API 非互換なし",
				"",
				"## 実施ステップ",
				"- 1. 実装",
			}, "\n"),
			wantParts: []string{"## 要件・仕様", "## 受け入れ条件", "## 制約事項・影響範囲"},
			noParts:   []string{"## 概要", "## 調査方針", "## 実施ステップ"},
		},
		{
			name: "英語見出しやスペースなし見出しにも対応する",
			issue: strings.Join([]string{
				"# Title",
				"",
				"##Overview",
				"skip",
				"",
				"## Requirements",
				"- keep requirement",
				"",
				"## Acceptance Criteria",
				"- keep acceptance",
				"",
				"## Constraints / Impact",
				"- keep impact",
			}, "\n"),
			wantParts: []string{"## Requirements", "## Acceptance Criteria", "## Constraints / Impact"},
			noParts:   []string{"##Overview"},
		},
		{
			name: "抽出結果が短すぎる場合は元本文にフォールバック",
			issue: strings.Join([]string{
				"# タイトル",
				"",
				"## 要件・仕様",
				"短い",
				"",
				"## 背景",
				strings.Repeat("背景説明", 120),
			}, "\n"),
			wantFull: true,
		},
		{
			name: "対象見出しが見つからない場合は元本文にフォールバック",
			issue: strings.Join([]string{
				"# タイトル",
				"",
				"## 概要",
				"overview",
				"",
				"## 目的・背景",
				"background",
			}, "\n"),
			wantFull: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := workflow.FilterIssueForCoder(tc.issue)
			if tc.wantFull {
				if got != tc.issue {
					t.Fatalf("want original markdown fallback, got:\n%s", got)
				}
				return
			}

			for _, part := range tc.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("filtered markdown should contain %q, got:\n%s", part, got)
				}
			}
			for _, part := range tc.noParts {
				if strings.Contains(got, part) {
					t.Errorf("filtered markdown should not contain %q, got:\n%s", part, got)
				}
			}
		})
	}
}

func TestApplyPatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(dir string) error // ファイルを事前に作成する処理
		patch     agent.FilePatch
		wantErr   bool
		wantErrIs string // エラーメッセージに含まれるべき文字列
		verify    func(t *testing.T, dir string)
	}{
		{
			name:  "Content で新規ファイルを作成する",
			patch: agent.FilePatch{Path: "new.go", Content: "package main\n"},
			verify: func(t *testing.T, dir string) {
				t.Helper()
				got, err := os.ReadFile(filepath.Join(dir, "new.go"))
				if err != nil {
					t.Fatalf("ファイルが作成されていません: %v", err)
				}
				if string(got) != "package main\n" {
					t.Errorf("content=%q; want %q", string(got), "package main\n")
				}
			},
		},
		{
			name: "Search/Replace で既存ファイルを修正する",
			setup: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "edit.go"), []byte("package main\n\nfunc Foo() {}\n"), 0o600)
			},
			patch: agent.FilePatch{Path: "edit.go", Search: "func Foo() {}", Replace: "func Bar() {}"},
			verify: func(t *testing.T, dir string) {
				t.Helper()
				got, err := os.ReadFile(filepath.Join(dir, "edit.go"))
				if err != nil {
					t.Fatalf("ファイルが読み込めません: %v", err)
				}
				if !strings.Contains(string(got), "func Bar() {}") {
					t.Errorf("置換が適用されていません: %s", string(got))
				}
				if strings.Contains(string(got), "func Foo() {}") {
					t.Errorf("置換前の文字列が残っています: %s", string(got))
				}
			},
		},
		{
			name: "Search が見つからない場合はエラーを返す",
			setup: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "noop.go"), []byte("package main\n"), 0o600)
			},
			patch:     agent.FilePatch{Path: "noop.go", Search: "func NotExist() {}", Replace: "func Bar() {}"},
			wantErr:   true,
			wantErrIs: "検索文字列が見つかりません",
		},
		{
			name:      "絶対パスはエラーを返す",
			patch:     agent.FilePatch{Path: "/etc/passwd", Content: "hacked"},
			wantErr:   true,
			wantErrIs: "絶対パスは許可されていません",
		},
		{
			name:      "パストラバーサルはエラーを返す",
			patch:     agent.FilePatch{Path: "../evil.go", Content: "package main"},
			wantErr:   true,
			wantErrIs: "パストラバーサル",
		},
		{
			name:      "content と search 両方が空の場合はエラーを返す",
			patch:     agent.FilePatch{Path: "empty.go"},
			wantErr:   true,
			wantErrIs: "content と search の両方が空",
		},
		{
			name: "行末空白のズレを正規化して置換する",
			setup: func(dir string) error {
				// 行末にスペースが付いている場合でも一致するべき。
				return os.WriteFile(filepath.Join(dir, "trail.go"), []byte("package main  \n\nfunc Old() {}\n"), 0o600)
			},
			patch: agent.FilePatch{Path: "trail.go", Search: "func Old() {}", Replace: "func New() {}"},
			verify: func(t *testing.T, dir string) {
				t.Helper()
				got, err := os.ReadFile(filepath.Join(dir, "trail.go"))
				if err != nil {
					t.Fatalf("ファイルが読み込めません: %v", err)
				}
				if !strings.Contains(string(got), "func New() {}") {
					t.Errorf("行末空白正規化後の置換が適用されていません: %s", string(got))
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if tc.setup != nil {
				if err := tc.setup(dir); err != nil {
					t.Fatalf("setup failed: %v", err)
				}
			}

			err := workflow.ApplyPatch(dir, tc.patch)
			if tc.wantErr {
				if err == nil {
					t.Fatal("エラーを期待しましたが nil でした")
				}
				if tc.wantErrIs != "" && !strings.Contains(err.Error(), tc.wantErrIs) {
					t.Errorf("エラーに %q が含まれるべき; got: %v", tc.wantErrIs, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.verify != nil {
				tc.verify(t, dir)
			}
		})
	}
}
