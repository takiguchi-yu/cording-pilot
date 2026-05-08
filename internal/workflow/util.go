package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/takiguchi-yu/cording-pilot/internal/agent"
	"github.com/takiguchi-yu/cording-pilot/internal/config"
)

// BuildProjectEnvHeader は設定ファイルのプロジェクト情報からプロンプト冒頭に挿入する
// 環境ヘッダー文字列を生成します。config が nil または言語が空の場合は空文字列を返します。
func BuildProjectEnvHeader(cfg *config.Config) string {
	if cfg == nil || cfg.Project.Language == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("【プロジェクト環境】\n")
	sb.WriteString(fmt.Sprintf("- 対象言語: %s\n", cfg.Project.Language))
	if cfg.Project.Framework != "" {
		sb.WriteString(fmt.Sprintf("- フレームワーク: %s\n", cfg.Project.Framework))
	}
	sb.WriteString("※ 実装・テストコードは必ずこの言語・フレームワークのベストプラクティスに従って記述してください。")
	return sb.String()
}

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

// ApplyPatch は workDir 配下の file.Path にパッチを適用します。
//
// file.Content が空でない場合: 新規ファイル（または上書き）として書き込みます。
// file.Search が空でない場合: 既存ファイルを読み込み、Search を Replace に1回だけ置換して保存します。
// 検索文字列が見つからない場合は、LLM が自己修復できるよう
// 「検索文字列が見つかりません」を含むエラーを返します。
func ApplyPatch(workDir string, file agent.FilePatch) error {
	if filepath.IsAbs(file.Path) {
		return fmt.Errorf("ApplyPatch: 絶対パスは許可されていません: %q", file.Path)
	}
	clean := filepath.Clean(file.Path)
	if strings.HasPrefix(clean, "..") {
		return fmt.Errorf("ApplyPatch: パストラバーサルが検出されました: %q", file.Path)
	}
	fullPath := filepath.Join(workDir, clean)
	rel, err := filepath.Rel(workDir, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("ApplyPatch: ワークディレクトリ外へのパスは許可されていません: %q", file.Path)
	}

	// 新規ファイル: Content が指定されている場合は safeWriteFile と同様に書き込む。
	if file.Content != "" {
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
			return fmt.Errorf("ApplyPatch: ディレクトリ作成に失敗しました: %w", err)
		}
		return os.WriteFile(fullPath, []byte(file.Content), 0o600)
	}

	// 既存ファイルのパッチ適用: Search/Replace が指定されている場合。
	if file.Search == "" {
		return fmt.Errorf("ApplyPatch: content と search の両方が空です: %q", file.Path)
	}

	existing, readErr := os.ReadFile(fullPath)
	if readErr != nil {
		return fmt.Errorf("ApplyPatch: ファイルの読み込みに失敗しました %q: %w", file.Path, readErr)
	}
	original := string(existing)

	// 完全一致で1回だけ置換する。
	if strings.Contains(original, file.Search) {
		replaced := strings.Replace(original, file.Search, file.Replace, 1)
		return os.WriteFile(fullPath, []byte(replaced), 0o600)
	}

	// 完全一致が失敗した場合: 行末空白のズレを吸収して再試行する。
	normalizedOriginal := normalizeLineEndings(original)
	normalizedSearch := normalizeLineEndings(file.Search)
	if strings.Contains(normalizedOriginal, normalizedSearch) {
		replaced := strings.Replace(normalizedOriginal, normalizedSearch, file.Replace, 1)
		return os.WriteFile(fullPath, []byte(replaced), 0o600)
	}

	return fmt.Errorf(
		"ApplyPatch: 検索文字列が見つかりません。path=%q, search=%q (LLM へのフィードバック: search フィールドの文字列をファイルの実際の内容と完全に一致させてください)",
		file.Path, file.Search,
	)
}

// normalizeLineEndings は各行の末尾空白・キャリッジリターンを除去し、
// 行末の空白ズレに起因するマッチ失敗を防ぎます。
func normalizeLineEndings(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r")
	}
	return strings.Join(lines, "\n")
}
