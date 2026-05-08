package workflow

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// TruncateLog は長大なログ出力を行数でスマートに切り詰めてトークン消費を抑制します。
// 総行数が maxLines を超える場合は、先頭10行と末尾(maxLines-10)行を残し、
// 中間に省略メッセージを挿入して返します。maxLines 以内の場合はそのまま返します。
func TruncateLog(output string, maxLines int) string {
	lines := strings.Split(output, "\n")
	if len(lines) <= maxLines {
		return output
	}
	headCount := 10
	if headCount >= maxLines {
		headCount = maxLines - 1
	}
	tailCount := maxLines - headCount
	snipped := len(lines) - headCount - tailCount
	head := lines[:headCount]
	tail := lines[len(lines)-tailCount:]
	return strings.Join(head, "\n") +
		fmt.Sprintf("\n... (snip %d lines) ...\n", snipped) +
		strings.Join(tail, "\n")
}

const (
	issueFilterMinRatio = 0.05
)

var coderKeepHeadingKeywords = []string{
	"要件仕様",
	"requirements",
	"requirement",
	"受け入れ条件",
	"受入条件",
	"acceptancecriteria",
	"制約事項影響範囲",
	"制約事項",
	"影響範囲",
	"constraintsimpact",
	"constraints",
	"impact",
}

var coderDropHeadingKeywords = []string{
	"概要",
	"目的背景",
	"目的",
	"背景",
	"調査方針",
	"調査手順",
	"方針決定基準",
	"実施ステップ",
	"overview",
	"motivation",
	"background",
	"investigation",
	"researchplan",
	"decisioncriteria",
	"implementationsteps",
	"steps",
}

// FilterIssueForCoder は、Issue Markdown から Coder に必要なセクションのみを抽出します。
// 抽出結果が空、または過度に短い場合は安全のため元の Markdown を返します。
func FilterIssueForCoder(issueMarkdown string) string {
	original := strings.TrimSpace(issueMarkdown)
	if original == "" {
		return issueMarkdown
	}

	lines := strings.Split(issueMarkdown, "\n")
	var filteredLines []string
	keepCurrentSection := false
	hasKeptSection := false

	for _, line := range lines {
		heading, ok := extractMarkdownHeadingText(line)
		if ok {
			keepCurrentSection = shouldKeepIssueSection(heading)
			if keepCurrentSection {
				hasKeptSection = true
				filteredLines = append(filteredLines, line)
			}
			continue
		}

		if keepCurrentSection {
			filteredLines = append(filteredLines, line)
		}
	}

	filtered := strings.TrimSpace(strings.Join(filteredLines, "\n"))
	if shouldFallbackToOriginal(original, filtered, hasKeptSection) {
		return issueMarkdown
	}

	return filtered
}

func extractMarkdownHeadingText(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || !strings.HasPrefix(trimmed, "#") {
		return "", false
	}

	headingLevel := 0
	for headingLevel < len(trimmed) && trimmed[headingLevel] == '#' {
		headingLevel++
	}
	if headingLevel == 0 || headingLevel > 6 {
		return "", false
	}

	heading := strings.TrimSpace(trimmed[headingLevel:])
	heading = strings.TrimSpace(strings.Trim(heading, "#"))
	if heading == "" {
		return "", false
	}

	return heading, true
}

func shouldKeepIssueSection(heading string) bool {
	normalized := normalizeHeadingText(heading)
	if normalized == "" {
		return false
	}

	for _, keyword := range coderDropHeadingKeywords {
		if strings.Contains(normalized, keyword) {
			return false
		}
	}

	for _, keyword := range coderKeepHeadingKeywords {
		if strings.Contains(normalized, keyword) {
			return true
		}
	}

	return false
}

func normalizeHeadingText(text string) string {
	normalized := strings.ToLower(strings.TrimSpace(text))
	remover := strings.NewReplacer(
		" ", "",
		"\t", "",
		"　", "",
		"-", "",
		"_", "",
		"/", "",
		"\\", "",
		":", "",
		"：", "",
		"・", "",
		"(", "",
		")", "",
		"（", "",
		"）", "",
	)
	return remover.Replace(normalized)
}

func shouldFallbackToOriginal(original, filtered string, hasKeptSection bool) bool {
	if !hasKeptSection || strings.TrimSpace(filtered) == "" {
		return true
	}

	originalLen := utf8.RuneCountInString(strings.TrimSpace(original))
	filteredLen := utf8.RuneCountInString(strings.TrimSpace(filtered))
	if originalLen == 0 {
		return false
	}

	ratio := float64(filteredLen) / float64(originalLen)
	return ratio < issueFilterMinRatio
}
