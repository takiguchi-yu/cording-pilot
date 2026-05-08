package workflow_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/takiguchi-yu/cording-pilot/internal/workflow"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

func TestLoadKnowledge_存在しないパスはWarnを出して継続する(t *testing.T) {
	t.Parallel()

	var logBuf bytes.Buffer
	log := logger.New(&logBuf)

	got := workflow.LoadKnowledge(log, t.TempDir(), []string{"missing-knowledge.md"})
	if got != "" {
		t.Fatalf("LoadKnowledge() = %q; want empty string", got)
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "\"level\":\"WARN\"") {
		t.Fatalf("WARN ログが出力されていません: %s", logOutput)
	}
	if !strings.Contains(logOutput, "knowledge.skip") {
		t.Fatalf("knowledge.skip ログが出力されていません: %s", logOutput)
	}
	if !strings.Contains(logOutput, "missing-knowledge.md") {
		t.Fatalf("対象パスがログに含まれていません: %s", logOutput)
	}
}
