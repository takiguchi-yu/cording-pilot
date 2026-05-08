package workflow

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

// LoadKnowledge は workDir を基点として paths のファイル・ディレクトリを読み込み、
// 結合したテキストを返します。ディレクトリの場合は .md と .txt のみ対象です。
// 存在しないパスや読み込みエラーは Warn ログを出力してスキップします。
func LoadKnowledge(log *logger.Logger, workDir string, paths []string) string {
	var sb strings.Builder
	for _, p := range paths {
		abs := p
		if !filepath.IsAbs(p) {
			abs = filepath.Join(workDir, p)
		}
		info, err := os.Stat(abs)
		if err != nil {
			_ = log.Warn("knowledge.skip", fmt.Sprintf("パスが存在しないためスキップします: %s (%v)", p, err))
			continue
		}
		if info.IsDir() {
			_ = filepath.WalkDir(abs, func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					_ = log.Warn("knowledge.walk_error", fmt.Sprintf("ディレクトリ探索エラー: %s (%v)", path, walkErr))
					return nil
				}
				if d.IsDir() {
					return nil
				}
				ext := strings.ToLower(filepath.Ext(path))
				if ext != ".md" && ext != ".txt" {
					return nil
				}
				appendKnowledgeFile(&sb, path, log)
				return nil
			})
		} else {
			appendKnowledgeFile(&sb, abs, log)
		}
	}
	return sb.String()
}

// appendKnowledgeFile はファイルを読み込み、ヘッダー付きで sb に追記します。
func appendKnowledgeFile(sb *strings.Builder, path string, log *logger.Logger) {
	data, err := os.ReadFile(path)
	if err != nil {
		_ = log.Warn("knowledge.read_error", fmt.Sprintf("ファイル読み込みエラー: %s (%v)", path, err))
		return
	}
	fmt.Fprintf(sb, "--- [%s] ---\n%s\n\n", path, string(data))
}
