package markdown_test

import (
	"testing"

	"github.com/takiguchi-yu/cording-pilot/pkg/markdown"
)

func TestExtractCodeBlock(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		content   string
		lang      string
		wantCode  string
		wantFound bool
	}{
		{
			name:      "Goブロックが見つかる",
			content:   "Here is code:\n```go\npackage main\n```\nend",
			lang:      "go",
			wantCode:  "package main",
			wantFound: true,
		},
		{
			name:      "フェンス後に改行なし_コードが直接抽出される",
			content:   "```gopackage main```",
			lang:      "go",
			wantCode:  "package main",
			wantFound: true,
		},
		{
			name:      "指定言語が存在しない",
			content:   "```python\nprint('hello')\n```",
			lang:      "go",
			wantFound: false,
		},
		{
			name:      "コンテンツが空",
			content:   "",
			lang:      "go",
			wantFound: false,
		},
		{
			name:      "閉じフェンスがないブロック",
			content:   "```go\npackage main",
			lang:      "go",
			wantFound: false,
		},
		{
			name:      "前後の空白がトリムされる",
			content:   "```go\n  package main  \n```",
			lang:      "go",
			wantCode:  "package main",
			wantFound: true,
		},
		{
			name:      "複数ブロックは最初のものを返す",
			content:   "```go\nfirst\n```\n```go\nsecond\n```",
			lang:      "go",
			wantCode:  "first",
			wantFound: true,
		},
		{
			name:      "異なる言語識別子",
			content:   "```bash\necho hello\n```",
			lang:      "bash",
			wantCode:  "echo hello",
			wantFound: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := markdown.ExtractCodeBlock(tc.content, tc.lang)
			if ok != tc.wantFound {
				t.Fatalf("found=%v; want %v", ok, tc.wantFound)
			}
			if ok && got != tc.wantCode {
				t.Errorf("code=%q; want %q", got, tc.wantCode)
			}
		})
	}
}
