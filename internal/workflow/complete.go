package workflow

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	githubpkg "github.com/takiguchi-yu/cording-pilot/internal/github"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

// CompleteState は ④ 完了フェーズ—成功終了ステートです。
// 標準出力にサマリを表示し、NDJSON ログをフラッシュし、隔離作業ディレクトリを削除します。
// GitHub が設定されている場合は、生成コードをリモートブランチへ push して PR を作成します。
type CompleteState struct {
	Logger *logger.Logger
	// GitHub は GitHub API クライアントです。nil の場合は GitHub 連携をスキップします。
	GitHub githubpkg.Client
	// GitHubToken は git push に使用する Personal Access Token です。
	GitHubToken string
	// RepoOwner はリポジトリのオーナー名です。
	RepoOwner string
	// RepoName はリポジトリ名です。
	RepoName string
	// BaseBranch はベースブランチ名（例: "main"）です。
	BaseBranch string
}

// Execute は State を実装します。
// ワークフローの終了を示すため常に (nil, nil) を返します。
func (s *CompleteState) Execute(ctx context.Context, wfCtx *Context) (State, error) {
	msg := fmt.Sprintf(
		"[COMPLETE] すべてのテストが %d 回の試行で通過しました。\n最後のテスト出力:\n%s",
		wfCtx.TryCount,
		wfCtx.LastTestOutput,
	)
	fmt.Println(msg)

	if err := s.Logger.Info("complete", msg); err != nil {
		return nil, err
	}

	// GitHub 連携が有効な場合は push と PR 作成を実行する。
	if s.GitHub != nil && s.GitHubToken != "" {
		if err := s.pushAndCreatePR(ctx, wfCtx); err != nil {
			// push/PR 失敗はワークフロー全体を失敗させない（警告ログのみ）。
			_ = s.Logger.Info("complete.push_warn", fmt.Sprintf("push/PR 作成に失敗しました（スキップ）: %v", err))
		}
	}

	return nil, nil
}

// pushAndCreatePR はリポジトリを shallow clone し、生成コードをブランチにコミットして
// push した後、GitHub API 経由で PR を作成します。
func (s *CompleteState) pushAndCreatePR(ctx context.Context, wfCtx *Context) error {
	// ── ブランチ名を生成 ─────────────────────────────────────────────────────
	branchName := generateBranchName(wfCtx.IssueNumber)
	wfCtx.BranchName = branchName

	if err := s.Logger.Info("complete.push_start", fmt.Sprintf("ブランチ %q への push を開始します", branchName)); err != nil {
		return err
	}

	// ── shallow clone ────────────────────────────────────────────────────────
	cloneDir, err := os.MkdirTemp("", "cording-pilot-clone-*")
	if err != nil {
		return fmt.Errorf("complete: create clone dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(cloneDir)
	}()

	remoteURL := fmt.Sprintf(
		"https://x-access-token:%s@github.com/%s/%s.git",
		s.GitHubToken, s.RepoOwner, s.RepoName,
	)

	base := s.BaseBranch
	if base == "" {
		base = "main"
	}

	if err = runGitCmd(ctx, "", "clone", "--depth=1", "--branch", base, remoteURL, cloneDir); err != nil {
		return fmt.Errorf("complete: git clone: %w", err)
	}

	// ── git 設定 ─────────────────────────────────────────────────────────────
	_ = runGitCmd(ctx, cloneDir, "config", "user.email", "cording-pilot@example.com")
	_ = runGitCmd(ctx, cloneDir, "config", "user.name", "Cording Pilot")

	// ── ブランチ作成 ──────────────────────────────────────────────────────────
	if err = runGitCmd(ctx, cloneDir, "checkout", "-b", branchName); err != nil {
		return fmt.Errorf("complete: git checkout: %w", err)
	}

	// ── origDir（元のローカルリポジトリルート）を取得 ─────────────────────────
	origDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("complete: get original dir: %w", err)
	}

	// ── 実装差分のみをコピー ──────────────────────────────────────────────────
	if err = copyImplementedFiles(origDir, wfCtx.WorkDir, cloneDir); err != nil {
		return fmt.Errorf("complete: copy generated files: %w", err)
	}

	// ── コミット ──────────────────────────────────────────────────────────────
	if err = runGitCmd(ctx, cloneDir, "add", "."); err != nil {
		return fmt.Errorf("complete: git add: %w", err)
	}

	commitMsg := buildCommitMessage(wfCtx.IssueNumber)
	if err = runGitCmd(ctx, cloneDir, "commit", "-m", commitMsg); err != nil {
		return fmt.Errorf("complete: git commit: %w", err)
	}

	// ── push ─────────────────────────────────────────────────────────────────
	if err = runGitCmd(ctx, cloneDir, "push", "origin", branchName); err != nil {
		return fmt.Errorf("complete: git push: %w", err)
	}

	if err = s.Logger.Info("complete.pushed", fmt.Sprintf("ブランチ %q を push しました", branchName)); err != nil {
		return err
	}

	// ── PR 作成 ───────────────────────────────────────────────────────────────
	title := buildPRTitle(wfCtx.IssueNumber)
	if wfCtx.IssueNumber > 0 {
		if issue, issueErr := s.GitHub.GetIssue(ctx, s.RepoOwner, s.RepoName, wfCtx.IssueNumber); issueErr == nil && issue.Title != "" {
			title = issue.Title
		}
	}
	body := buildPRBody(wfCtx.IssueNumber, origDir)
	pr, err := s.GitHub.CreatePullRequest(ctx, s.RepoOwner, s.RepoName, title, branchName, base, body)
	if err != nil {
		return fmt.Errorf("complete: create PR: %w", err)
	}

	return s.Logger.Info(
		"complete.pr_created",
		fmt.Sprintf("PR #%d を作成しました: %s", pr.Number, pr.URL),
	)
}

// generateBranchName は IssueNumber からブランチ名を生成します。
// タイムスタンプを付与することで、同一 Issue を再実行した際のブランチ名競合を回避します。
func generateBranchName(issueNumber int) string {
	ts := time.Now().Format("20060102150405")
	if issueNumber > 0 {
		return fmt.Sprintf("issue-%d/implement-%s", issueNumber, ts)
	}
	return fmt.Sprintf("cording-pilot/%s", ts)
}

// buildCommitMessage は IssueNumber に基づいてコミットメッセージを生成します。
func buildCommitMessage(issueNumber int) string {
	if issueNumber > 0 {
		return fmt.Sprintf("feat: Issue #%d の実装", issueNumber)
	}
	return "feat: 実装"
}

// buildPRTitle は IssueNumber に基づいて PR タイトルを生成します。
func buildPRTitle(issueNumber int) string {
	if issueNumber > 0 {
		return fmt.Sprintf("Issue #%d の実装", issueNumber)
	}
	return "実装"
}

// runGitCmd は指定ディレクトリで git コマンドを実行します。
// dir が空文字の場合はカレントディレクトリで実行します。
func runGitCmd(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 — args は内部で構築
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v: %w\n%s", args, err, out)
	}
	return nil
}

// isExcludedRel は rel パスが除外対象（.git, .worktrees, .direnv）かどうかを返します。
func isExcludedRel(rel string) bool {
	for _, ex := range []string{".git", ".worktrees", ".direnv"} {
		if rel == ex || strings.HasPrefix(rel, ex+"/") {
			return true
		}
	}
	return false
}

// fileContentDiffers はファイル a と b の内容が異なるかどうかを返します。
// 呼び出し前にファイルサイズで事前フィルタを行うことを推奨します。
func fileContentDiffers(a, b string) (bool, error) {
	ab, err := os.ReadFile(a) // #nosec G304 — 内部で構築したパス
	if err != nil {
		return false, fmt.Errorf("fileContentDiffers: read a: %w", err)
	}
	bb, err := os.ReadFile(b) // #nosec G304 — 内部で構築したパス
	if err != nil {
		return false, fmt.Errorf("fileContentDiffers: read b: %w", err)
	}
	return !bytes.Equal(ab, bb), nil
}

// copyImplementedFiles は origDir と workDir を比較し、AI が変更・追加・削除した
// ファイルのみを dst に反映します。
// 変更・追加ファイルは dst にコピーし、AI が削除したファイルは dst から os.Remove します。
// .git, .worktrees, .direnv はすべての操作でスキップします。
func copyImplementedFiles(origDir, workDir, dst string) error {
	// ── Phase 1: workDir を歩いて変更・追加ファイルを dst にコピー ────────────
	if err := filepath.WalkDir(workDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(workDir, path)
		if err != nil {
			return fmt.Errorf("copyImplementedFiles: rel: %w", err)
		}

		if isExcludedRel(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		destPath := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0o755)
		}

		origPath := filepath.Join(origDir, rel)
		origInfo, statErr := os.Stat(origPath)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				// origDir に存在しない → AI が新規追加したファイル
				return copyFile(path, destPath)
			}
			return fmt.Errorf("copyImplementedFiles: stat orig: %w", statErr)
		}

		// サイズで事前判定（巨大ファイルの全量読み込みを抑制）
		workInfo, infoErr := d.Info()
		if infoErr != nil {
			return fmt.Errorf("copyImplementedFiles: info work: %w", infoErr)
		}
		if origInfo.Size() != workInfo.Size() {
			// サイズ相違 → 変更あり
			return copyFile(path, destPath)
		}

		// 内容の比較
		differs, cmpErr := fileContentDiffers(origPath, path)
		if cmpErr != nil {
			return fmt.Errorf("copyImplementedFiles: compare: %w", cmpErr)
		}
		if differs {
			return copyFile(path, destPath)
		}

		return nil
	}); err != nil {
		return err
	}

	// ── Phase 2: origDir を歩いて workDir に存在しないファイルを dst から削除 ──
	return filepath.WalkDir(origDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(origDir, path)
		if err != nil {
			return fmt.Errorf("copyImplementedFiles: rel orig: %w", err)
		}

		if isExcludedRel(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		// workDir に存在しない → AI が削除したファイル
		workPath := filepath.Join(workDir, rel)
		if _, statErr := os.Stat(workPath); os.IsNotExist(statErr) {
			dstPath := filepath.Join(dst, rel)
			if removeErr := os.Remove(dstPath); removeErr != nil && !os.IsNotExist(removeErr) { // #nosec G304
				return fmt.Errorf("copyImplementedFiles: remove: %w", removeErr)
			}
		}

		return nil
	})
}

// buildPRBody はリポジトリのPRテンプレートを読み込み、Issue への紐付けキーワードを付与して
// PR 本文を生成します。テンプレートが存在しない場合はデフォルトメッセージにフォールバックします。
func buildPRBody(issueNumber int, dir string) string {
	templatePaths := []string{
		".github/pull_request_template.md",
		".github/PULL_REQUEST_TEMPLATE.md",
		"pull_request_template.md",
	}

	var body string
	for _, relPath := range templatePaths {
		data, err := os.ReadFile(filepath.Join(dir, relPath)) // #nosec G304
		if err == nil {
			body = string(data)
			break
		}
	}

	if body == "" {
		body = "Cording Pilot による自動生成 PR です。"
	}

	if issueNumber > 0 {
		if !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		body += fmt.Sprintf("\n---\n\nCloses #%d\n", issueNumber)
	}

	return body
}

// copyDir は src ディレクトリの内容を dst ディレクトリへ再帰的にコピーします。
// VCS 管理用ディレクトリ（.git と .worktrees）および開発環境依存ディレクトリ（.direnv）はスキップします。
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("copyDir: rel path: %w", err)
		}

		if isExcludedRel(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		dest := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}

		return copyFile(path, dest)
	})
}

// copyFile は src ファイルを dst にコピーします。
func copyFile(src, dst string) error {
	in, err := os.Open(src) // #nosec G304 — src はワークフロー内部で生成したパス
	if err != nil {
		return fmt.Errorf("copyFile: open src: %w", err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst) // #nosec G304 — dst は内部で構築したパス
	if err != nil {
		return fmt.Errorf("copyFile: create dst: %w", err)
	}
	defer func() { _ = out.Close() }()

	if _, err = io.Copy(out, in); err != nil {
		return fmt.Errorf("copyFile: copy: %w", err)
	}
	return nil
}
