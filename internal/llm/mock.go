package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// mockFile は GenerateStructured が返す JSON 内のファイルエントリです。
type mockFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// mockCodeResult は GenerateStructured が返す JSON の最上位オブジェクトです。
type mockCodeResult struct {
	Files []mockFile `json:"files"`
}

// mockQuestion は ClarificationRequest 内の単一質問を表します。
type mockQuestion struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	Type    string   `json:"type"`
	Options []string `json:"options"`
}

// mockClarificationResultType は GenerateStructured が [CLARIFY] キーワードで返す型です。
type mockClarificationResultType struct {
	Questions []mockQuestion `json:"questions"`
	IsClear   bool           `json:"is_clear"`
}

// mockClarificationResult は [CLARIFY] キーワードへのデフォルトレスポンスです。
var mockClarificationResult = mockClarificationResultType{
	IsClear: false,
	Questions: []mockQuestion{
		{
			ID:   "scope",
			Text: "実装のスコープを教えてください（新規機能 / 既存機能の改修 / バグ修正）",
			Type: "select",
			Options: []string{
				"新規機能",
				"既存機能の改修",
				"バグ修正",
			},
		},
		{
			ID:   "test_required",
			Text: "テストコードも一緒に生成しますか？",
			Type: "confirm",
		},
		{
			ID:   "additional_notes",
			Text: "その他の補足事項があれば入力してください（任意）",
			Type: "text",
		},
	},
}

const mockCompiledIssue = `# 文字列を逆順にする関数を実装する

## 概要 (Overview)

ユーザーのヒアリング結果を元に要件を確定しました。

## 要件・仕様 (Requirements)

- 入力: 任意の文字列（UTF-8）
- 出力: UTF-8文字単位で逆順にした文字列
- エッジケース: 空文字、1文字、マルチバイト文字
- テストコード: あり

## 制約事項・影響範囲 (Constraints/Impact)

- 新規ファイル: task.go / task_test.go`

const mockTestCodeContent = `package task

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
`

const mockBuggyImplContent = `package task

// Reverse is a stub that intentionally returns the input unchanged.
func Reverse(s string) string {
	return s
}
`

const mockCorrectImplContent = `package task

// Reverse returns the UTF-8 characters of s in reverse order.
func Reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
`

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

// MockClient は Client のモック実装です。ローカル開発・プロトタイピング用です。
// プロンプト中のキーワードに基づいて事前定義済みのレスポンスを返します。
// 最初の実装リクエストではバグのあるコードを返し、2 回目以降は正しい実装を返すことで
// Fix Loop の収束をシミュレートします。
type MockClient struct {
	implCallCount int
}

// NewMockClient は新しい MockClient を生成します。
func NewMockClient() *MockClient {
	return &MockClient{}
}

// taskPart はプロンプトから「---」セパレータ以降のタスク部分を抽出します。
func taskPart(prompt string) string {
	const sep = "\n\n---\n\n"
	if idx := strings.Index(prompt, sep); idx != -1 {
		return prompt[idx+len(sep):]
	}
	return prompt
}

// Generate はプロンプト中のキーワードに基づいた事前定義済みのテキストレスポンスを返します。
// プロンプトにシステム／タスクセパレータ "---" が含まれる場合、タスク部分のみでキーワードを判定します。
func (m *MockClient) Generate(_ context.Context, prompt string) (string, error) {
	lower := strings.ToLower(taskPart(prompt))

	switch {
	case strings.Contains(lower, "[review]") || strings.Contains(lower, "レビュー"):
		return mockReviewApprove, nil
	case strings.Contains(lower, "[plan]") || strings.Contains(lower, "実装計画を作成"):
		return mockPlanText, nil
	case strings.Contains(lower, "[compile_issue]"):
		return mockCompiledIssue, nil
	default:
		return "", nil
	}
}

// GenerateStructured はプロンプト中のキーワードに基づいた JSON を target にデコードします。
// [CLARIFY] キーワードには ClarificationRequest を、それ以外には CodeGenerationResult 相当の
// JSON を返します。JSON デコードに失敗した場合は ErrJSONParse をラップしたエラーを返します。
func (m *MockClient) GenerateStructured(_ context.Context, prompt string, target interface{}) error {
	lower := strings.ToLower(taskPart(prompt))

	if strings.Contains(lower, "[clarify]") {
		result := mockClarificationResult
		data, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("%w: marshal clarification: %v", ErrJSONParse, err)
		}
		if err = json.Unmarshal(data, target); err != nil {
			return fmt.Errorf("%w: %v", ErrJSONParse, err)
		}
		return nil
	}

	var result mockCodeResult
	if strings.Contains(lower, "[test_gen]") {
		result = mockCodeResult{
			Files: []mockFile{
				{Path: "task_test.go", Content: mockTestCodeContent},
			},
		}
	} else {
		m.implCallCount++
		content := mockBuggyImplContent
		if m.implCallCount > 1 {
			content = mockCorrectImplContent
		}
		result = mockCodeResult{
			Files: []mockFile{
				{Path: "task.go", Content: content},
			},
		}
	}

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("%w: marshal mock result: %v", ErrJSONParse, err)
	}
	if err = json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("%w: %v", ErrJSONParse, err)
	}
	return nil
}
