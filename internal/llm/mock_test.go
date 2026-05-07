package llm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/takiguchi-yu/cording-pilot/internal/llm"
	"github.com/takiguchi-yu/cording-pilot/pkg/markdown"
)

func TestMockClient_PLANキーワードで計画レスポンスを返す(t *testing.T) {
	t.Parallel()
	c := llm.NewMockClient()
	resp, err := c.Generate(context.Background(), "[PLAN] 実装計画を作成してください。\n\nsome req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == "" {
		t.Error("expected non-empty plan response")
	}
}

func TestMockClient_TEST_GENキーワードでテストコードを返す(t *testing.T) {
	t.Parallel()
	c := llm.NewMockClient()
	resp, err := c.Generate(context.Background(), "dummy system\n\n---\n\n[TEST_GEN] テストコードを生成してください")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	code, ok := markdown.ExtractCodeBlock(resp, "go")
	if !ok {
		t.Fatalf("response should contain a Go code block; got: %q", resp)
	}
	if !strings.Contains(code, "TestReverse") {
		t.Errorf("test code should contain TestReverse; got: %q", code)
	}
}

func TestMockClient_REVIEWキーワードで承認レスポンスを返す(t *testing.T) {
	t.Parallel()
	c := llm.NewMockClient()
	resp, err := c.Generate(context.Background(), "dummy system\n\n---\n\n[REVIEW] レビューしてください")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(strings.ToLower(resp), "approve") {
		t.Errorf("review response should contain 'approve'; got: %q", resp)
	}
}

func TestMockClient_初回実装呼び出しでバグのあるコードを返す(t *testing.T) {
	t.Parallel()
	c := llm.NewMockClient()
	resp, err := c.Generate(context.Background(), "implement something")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	code, ok := markdown.ExtractCodeBlock(resp, "go")
	if !ok {
		t.Fatalf("expected Go code block; got: %q", resp)
	}
	// The buggy stub returns the input unchanged.
	if strings.Contains(code, "runes") {
		t.Errorf("first impl call should return buggy code (no rune reversal); got: %q", code)
	}
}

func TestMockClient_2回目実装呼び出しで正しいコードを返す(t *testing.T) {
	t.Parallel()
	c := llm.NewMockClient()

	// First call: buggy.
	if _, err := c.Generate(context.Background(), "implement something"); err != nil {
		t.Fatalf("first call error: %v", err)
	}

	// Second call: correct implementation.
	resp, err := c.Generate(context.Background(), "implement something again")
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	code, ok := markdown.ExtractCodeBlock(resp, "go")
	if !ok {
		t.Fatalf("expected Go code block; got: %q", resp)
	}
	if !strings.Contains(code, "rune") {
		t.Errorf("second impl call should return correct code with rune handling; got: %q", code)
	}
}
