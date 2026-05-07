package llm

import (
	"context"
	"strings"
)

const mockTestCode = "```go\n" + `package task

import "testing"

func TestReverse(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"hello", "olleh"},
		{"world", "dlrow"},
		{"", ""},
		{"a", "a"},
		{"日本語", "語本日"},
	}
	for _, tc := range cases {
		got := Reverse(tc.input)
		if got != tc.want {
			t.Errorf("Reverse(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}
` + "```"

const mockBuggyImplCode = "```go\n" + `package task

// Reverse is a stub that intentionally returns the input unchanged.
func Reverse(s string) string {
	return s
}
` + "```"

const mockCorrectImplCode = "```go\n" + `package task

// Reverse returns the UTF-8 characters of s in reverse order.
func Reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
` + "```"

const mockPlanText = `## 実装計画

### 概要
文字列を逆順にする関数 Reverse を実装する。

### 仕様
- 入力: 任意の文字列（UTF-8）
- 出力: UTF-8文字単位で逆順にした文字列
- エッジケース: 空文字、1文字、マルチバイト文字

### 影響範囲
- 新規ファイル: task.go / task_test.go`

const mockReviewApprove = `## レビュー結果: Approve

すべてのテストが通過しており、実装は要件を満たしています。
コードスタイルも Effective Go に準拠しています。`

// MockClient is a mock implementation of Client for local development and prototyping.
// It dispatches pre-canned responses based on keywords in the prompt.
// On the first implementation request it returns buggy code; subsequent calls
// return the correct implementation, simulating Fix Loop convergence.
type MockClient struct {
	implCallCount int
}

// NewMockClient creates a new MockClient.
func NewMockClient() *MockClient {
	return &MockClient{}
}

// Generate returns a pre-defined response determined by keywords in prompt.
// If the prompt contains the system/task separator "---", only the task section
// (the part after the separator) is inspected for dispatch keywords.
func (m *MockClient) Generate(_ context.Context, prompt string) (string, error) {
	// Use only the task portion of the prompt for dispatch so that keywords
	// present in the system prompt do not interfere with routing.
	taskPart := prompt
	const sep = "\n\n---\n\n"
	if idx := strings.Index(prompt, sep); idx != -1 {
		taskPart = prompt[idx+len(sep):]
	}
	lower := strings.ToLower(taskPart)

	switch {
	case strings.Contains(lower, "[review]") || strings.Contains(lower, "レビュー"):
		return mockReviewApprove, nil
	case strings.Contains(lower, "[test_gen]"):
		return mockTestCode, nil
	case strings.Contains(lower, "[plan]") || strings.Contains(lower, "実装計画を作成"):
		return mockPlanText, nil
	default:
		m.implCallCount++
		if m.implCallCount == 1 {
			return mockBuggyImplCode, nil
		}
		return mockCorrectImplCode, nil
	}
}
