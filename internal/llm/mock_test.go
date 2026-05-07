package llm_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/takiguchi-yu/cording-pilot/internal/llm"
)

// codeResult は GenerateStructured がデコードする最上位構造です。
type codeResult struct {
	Files []struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	} `json:"files"`
}

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

func TestMockClient_GenerateStructured_TEST_GENでテストコードを返す(t *testing.T) {
	t.Parallel()
	c := llm.NewMockClient()
	var result codeResult
	err := c.GenerateStructured(context.Background(), "dummy system\n\n---\n\n[TEST_GEN] テストコードを生成してください", &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatal("expected at least one file in result")
	}
	if result.Files[0].Path != "task_test.go" {
		t.Errorf("expected path=task_test.go; got %q", result.Files[0].Path)
	}
	if !strings.Contains(result.Files[0].Content, "TestReverse") {
		t.Errorf("test code should contain TestReverse; got: %q", result.Files[0].Content)
	}
}

func TestMockClient_GenerateStructured_初回実装呼び出しでバグのあるコードを返す(t *testing.T) {
	t.Parallel()
	c := llm.NewMockClient()
	var result codeResult
	if err := c.GenerateStructured(context.Background(), "implement something", &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatal("expected at least one file")
	}
	// The buggy stub returns the input unchanged.
	if strings.Contains(result.Files[0].Content, "runes") {
		t.Errorf("first impl call should return buggy code (no rune reversal); got: %q", result.Files[0].Content)
	}
}

func TestMockClient_GenerateStructured_2回目実装呼び出しで正しいコードを返す(t *testing.T) {
	t.Parallel()
	c := llm.NewMockClient()

	// First call: buggy.
	if err := c.GenerateStructured(context.Background(), "implement something", &codeResult{}); err != nil {
		t.Fatalf("first call error: %v", err)
	}

	// Second call: correct implementation.
	var result codeResult
	if err := c.GenerateStructured(context.Background(), "implement something again", &result); err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatal("expected at least one file")
	}
	if !strings.Contains(result.Files[0].Content, "rune") {
		t.Errorf("second impl call should return correct code with rune handling; got: %q", result.Files[0].Content)
	}
}

func TestMockClient_GenerateStructured_JSONとして正常にデコードできる(t *testing.T) {
	t.Parallel()
	c := llm.NewMockClient()

	// target に *json.RawMessage を使ってデコード確認する。
	var raw json.RawMessage
	if err := c.GenerateStructured(context.Background(), "[TEST_GEN] something", &raw); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !json.Valid(raw) {
		t.Errorf("result is not valid JSON: %s", raw)
	}
}
