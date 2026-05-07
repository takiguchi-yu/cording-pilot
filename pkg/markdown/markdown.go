// Package markdown は Markdown 形式のテキストを解析するユーティリティを提供します。
package markdown

import "strings"

// ExtractCodeBlock は Markdown 文字列から、指定した言語識別子に一致する
// 最初のコードフェンスブロックを抽出します。
//
// 例えば lang を "go" に指定した場合、```go … ``` で囲まれたブロックを探します。
// 抽出に成功した場合は前後の空白を除いたコード文字列と true を返します。
// 対象ブロックが見つからない場合は空文字列と false を返します。
func ExtractCodeBlock(content, lang string) (string, bool) {
	openFence := "```" + lang
	closeFence := "```"

	start := strings.Index(content, openFence)
	if start == -1 {
		return "", false
	}

	codeStart := start + len(openFence)
	if codeStart < len(content) && content[codeStart] == '\n' {
		codeStart++
	}

	end := strings.Index(content[codeStart:], closeFence)
	if end == -1 {
		return "", false
	}

	return strings.TrimSpace(content[codeStart : codeStart+end]), true
}
