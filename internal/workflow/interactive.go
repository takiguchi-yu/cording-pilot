package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/takiguchi-yu/cording-pilot/internal/agent"
	githubpkg "github.com/takiguchi-yu/cording-pilot/internal/github"
	"github.com/takiguchi-yu/cording-pilot/internal/tui"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

const (
	issueTypeFeature = "feature"
	issueTypeBug     = "bug"
)

// InteractiveState は ⓪ 対話フェーズです。
// Planner Agent が要件の不足を分析して質問リストを生成し、TUI フォームでユーザーへ提示します。
// ユーザーの回答を元に最終的な実装計画（擬似 Issue）を生成し、wfCtx に保存して Next へ遷移します。
//
// GitHub が設定されている場合:
//   - wfCtx.IssueNumber > 0 なら GetIssue で本文を取得して Requirement に設定する。
//   - 新規要件の場合は CompileIssue 後に CreateIssue を実行し、IssueNumber を採番する。
type InteractiveState struct {
	// Planner は要件の分析と Issue のコンパイルを担当する計画エージェントです。
	Planner agent.PlannerAgent
	// Logger は処理ログを出力する NDJSON ロガーです。
	Logger *logger.Logger
	// Next は後継ステート（通常は PlanState）です。
	Next State
	// GitHub は GitHub API クライアントです。nil の場合は GitHub 連携をスキップします。
	GitHub githubpkg.Client
	// RepoOwner はリポジトリのオーナー名です。
	RepoOwner string
	// RepoName はリポジトリ名です。
	RepoName string
	// SelectIssueType は Issue 種別の選択関数です。nil の場合はデフォルトの TUI を使用します。
	SelectIssueType func() (string, error)
	// LoadIssueTemplate は Issue テンプレートを読み込む関数です。nil の場合はデフォルトのローダーを使用します。
	LoadIssueTemplate func(issueType string) (string, error)
}

// Execute implements State.
//
// LLM との通信には ctx（タイムアウトあり）を使用し、
// TUI フォームの入力待機には context.Background()（タイムアウトなし）を使用します。
// これにより、ユーザーの入力中に LLM タイムアウトが発火してフォームが中断されることを防ぎます。
func (s *InteractiveState) Execute(ctx context.Context, wfCtx *Context) (State, error) {
	if err := s.Logger.Info("interactive.start", "⓪ 対話フェーズを開始します"); err != nil {
		return nil, err
	}

	// ── Step 0: Issue 番号が指定されている場合は GitHub から本文を取得 ──────────────
	if wfCtx.IssueNumber > 0 && s.GitHub != nil {
		return s.fetchIssue(ctx, wfCtx)
	}

	// ── Step 0.5: Issue 種別を選択してテンプレートを読み込む ─────────────────────
	issueType, err := s.selectIssueType()
	if err != nil {
		if errors.Is(err, tui.ErrAborted) {
			return nil, fmt.Errorf("interactive: %w", tui.ErrAborted)
		}
		return nil, fmt.Errorf("interactive: select issue type: %w", err)
	}

	templateContent, err := s.loadIssueTemplate(issueType)
	if err != nil {
		templateContent = ""
		if logErr := s.Logger.Warn("interactive.template_load_failed", fmt.Sprintf("Issue テンプレートの読み込みに失敗しました: %v", err)); logErr != nil {
			return nil, logErr
		}
	}

	// ── Step 1: 要件の不足を分析して質問リストを生成 ────────────────────────────
	clarification, err := s.Planner.GenerateClarification(ctx, wfCtx.Requirement)
	if err != nil {
		return nil, fmt.Errorf("interactive: generate clarification: %w", err)
	}
	questions := clarification.Questions
	if len(questions) > 1 {
		questions = questions[:1]
		if err = s.Logger.Info("interactive.questions_trimmed", "確認事項は最大1件に制限されるため、先頭の質問のみを使用します"); err != nil {
			return nil, err
		}
	}

	answers := map[string]string{}

	// ── Step 2: 質問が生成されなかった場合のみ TUI をスキップ ───────────────────────
	// IsClear は参照しない。自然言語が渡された場合でも必ずヒアリングを実施するため、
	// LLM が質問を生成しなかった（Questions が空）ときのみスキップする。
	if len(questions) == 0 {
		if err = s.Logger.Info("interactive.skip", "要件が十分明確なため、ヒアリングをスキップします"); err != nil {
			return nil, err
		}
	} else {
		if err = s.Logger.Info("interactive.questions", fmt.Sprintf("%d 件の確認事項を生成しました", len(questions))); err != nil {
			return nil, err
		}

		// ── Step 3: TUI フォームでユーザーへヒアリング ─────────────────────────────
		// TUI の入力待機は context.Background() で行い、LLM タイムアウトの影響を受けないようにします。
		answers, err = tui.RunForm(questions)
		if err != nil {
			if errors.Is(err, tui.ErrAborted) {
				return nil, fmt.Errorf("interactive: %w", tui.ErrAborted)
			}
			return nil, fmt.Errorf("interactive: form: %w", err)
		}

		if err = s.Logger.Info("interactive.answers_collected", fmt.Sprintf("%d 件の回答を収集しました", len(answers))); err != nil {
			return nil, err
		}
	}

	// ── Step 4: 回答を元に最終的な実装計画（Issue）をコンパイル ─────────────────
	compiled, err := s.Planner.CompileIssue(ctx, wfCtx.Requirement, answers, templateContent)
	if err != nil {
		return nil, fmt.Errorf("interactive: compile issue: %w", err)
	}

	wfCtx.Requirement = compiled

	if err = s.Logger.Info("interactive.done", compiled); err != nil {
		return nil, err
	}

	// ── Step 5: GitHub Issue を作成 ──────────────────────────────────────────
	if err = s.maybeCreateIssue(ctx, wfCtx); err != nil {
		return nil, err
	}

	return s.Next, nil
}

func (s *InteractiveState) selectIssueType() (string, error) {
	if s.SelectIssueType != nil {
		return s.SelectIssueType()
	}

	issueType := issueTypeFeature
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Issue 種別を選択してください").
				Description("開始するタスクに合わせてテンプレートを切り替えます").
				Options(
					huh.NewOption("Feature (新規実装)", issueTypeFeature),
					huh.NewOption("Bug (バグ対応)", issueTypeBug),
				).
				Value(&issueType),
		),
	)

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", tui.ErrAborted
		}
		return "", fmt.Errorf("interactive: run issue type select: %w", err)
	}

	return issueType, nil
}

func (s *InteractiveState) loadIssueTemplate(issueType string) (string, error) {
	if s.LoadIssueTemplate != nil {
		return s.LoadIssueTemplate(issueType)
	}

	if issueType == "" {
		return "", nil
	}

	path := filepath.Join(".github", "ISSUE_TEMPLATE", issueType+".md")
	data, err := os.ReadFile(path) // #nosec G304 -- issueType は固定選択肢から選ばれる
	if err != nil {
		return "", fmt.Errorf("interactive: read template %q: %w", path, err)
	}

	return string(data), nil
}

// fetchIssue は GitHub から Issue を取得し、本文を Requirement に設定します。
func (s *InteractiveState) fetchIssue(ctx context.Context, wfCtx *Context) (State, error) {
	if err := s.Logger.Info(
		"interactive.fetch_issue",
		fmt.Sprintf("Issue #%d を GitHub から取得します", wfCtx.IssueNumber),
	); err != nil {
		return nil, err
	}

	issue, err := s.GitHub.GetIssue(ctx, s.RepoOwner, s.RepoName, wfCtx.IssueNumber)
	if err != nil {
		return nil, fmt.Errorf("interactive: fetch issue #%d: %w", wfCtx.IssueNumber, err)
	}

	wfCtx.Requirement = fmt.Sprintf("# %s\n\n%s", issue.Title, issue.Body)

	if err = s.Logger.Info(
		"interactive.issue_fetched",
		fmt.Sprintf("Issue #%d を取得しました: %s", issue.Number, issue.Title),
	); err != nil {
		return nil, err
	}

	return s.Next, nil
}

// maybeCreateIssue は GitHub クライアントが設定されている場合に Issue を作成します。
func (s *InteractiveState) maybeCreateIssue(ctx context.Context, wfCtx *Context) error {
	if s.GitHub == nil || wfCtx.IssueNumber > 0 {
		return nil
	}

	title, body := splitIssueText(wfCtx.Requirement)
	issue, err := s.GitHub.CreateIssue(ctx, s.RepoOwner, s.RepoName, title, body)
	if err != nil {
		if logErr := s.Logger.Warn("interactive.issue_create_failed", fmt.Sprintf("Issue の作成に失敗しました: %v", err)); logErr != nil {
			return logErr
		}
		return fmt.Errorf("interactive: create issue: %w", err)
	}

	wfCtx.IssueNumber = issue.Number
	return s.Logger.Info(
		"interactive.issue_created",
		fmt.Sprintf("Issue #%d を作成しました: %s", issue.Number, issue.Title),
	)
}

// splitIssueText は Markdown テキストから Issue タイトルと本文を分割します。
// 先頭の H1 見出し（# title）がある場合はタイトルに採用し、残りを本文にします。
// H1 がない場合は本文の内容からタイトルを推定し、本文はそのまま使用します。
func splitIssueText(text string) (title, body string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "実装タスク", ""
	}

	if h1Title, h1Body, ok := splitByLeadingH1(trimmed); ok {
		return h1Title, h1Body
	}

	return deriveIssueTitle(trimmed), trimmed
}

func splitByLeadingH1(text string) (title, body string, ok bool) {
	lines := strings.Split(text, "\n")
	firstLineIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			firstLineIdx = i
			break
		}
	}

	if firstLineIdx < 0 {
		return "", "", false
	}

	firstLine := strings.TrimSpace(lines[firstLineIdx])
	if !strings.HasPrefix(firstLine, "# ") {
		return "", "", false
	}

	title = strings.TrimSpace(strings.TrimPrefix(firstLine, "# "))
	body = strings.TrimSpace(strings.Join(lines[firstLineIdx+1:], "\n"))
	if title == "" {
		return "", "", false
	}

	return title, body, true
}

func deriveIssueTitle(text string) string {
	if firstContent := firstMeaningfulLine(text); firstContent != "" {
		return truncateRunes(firstContent, 80)
	}

	return "実装タスク"
}

func firstMeaningfulLine(text string) string {
	lines := strings.Split(text, "\n")
	inComment := false
	inCodeFence := false

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "```") {
			inCodeFence = !inCodeFence
			continue
		}
		if inCodeFence {
			continue
		}

		if inComment {
			if strings.Contains(line, "-->") {
				inComment = false
			}
			continue
		}
		if strings.HasPrefix(line, "<!--") {
			if !strings.Contains(line, "-->") {
				inComment = true
			}
			continue
		}

		if strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") {
			line = strings.TrimSpace(line[2:])
		}

		if trimmed, ok := trimOrderedListPrefix(line); ok {
			line = trimmed
		}

		line = strings.TrimSpace(strings.Trim(line, "*_`>"))
		if line == "" {
			continue
		}

		return line
	}

	return ""
}

func trimOrderedListPrefix(line string) (string, bool) {
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 || i+1 >= len(line) || line[i] != '.' || line[i+1] != ' ' {
		return line, false
	}
	return strings.TrimSpace(line[i+2:]), true
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
