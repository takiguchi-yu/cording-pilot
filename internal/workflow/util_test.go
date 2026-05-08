package workflow_test

import (
	"strings"
	"testing"

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
