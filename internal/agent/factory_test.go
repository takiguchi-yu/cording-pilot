package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/takiguchi-yu/cording-pilot/internal/agent"
	"github.com/takiguchi-yu/cording-pilot/internal/config"
)

// stubLLM はこのテストパッケージ専用の最小限 llm.Client スタブです。
type stubLLM struct {
	response           string
	err                error
	lastPrompt         string
	structuredResponse interface{}
	structuredErr      error
}

func (s *stubLLM) Generate(_ context.Context, prompt string) (string, error) {
	s.lastPrompt = prompt
	return s.response, s.err
}

func (s *stubLLM) GenerateStructured(_ context.Context, prompt string, target interface{}) error {
	s.lastPrompt = prompt
	if s.structuredErr != nil {
		return s.structuredErr
	}
	data, err := json.Marshal(s.structuredResponse)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func TestFactory_NewPlanner_Askを呼び出す(t *testing.T) {
	t.Parallel()
	stub := &stubLLM{response: "plan output"}
	f := agent.NewFactory(stub, config.DefaultGoConfig())
	planner := f.NewPlannerAgent()

	resp, err := planner.Ask(context.Background(), "要件: 文字列を逆順にする")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "plan output" {
		t.Errorf("response=%q; want %q", resp, "plan output")
	}
	// The system prompt and user task should both appear in the forwarded prompt.
	if !strings.Contains(stub.lastPrompt, "要件: 文字列を逆順にする") {
		t.Errorf("user task not forwarded to LLM; prompt=%q", stub.lastPrompt)
	}
}

func TestFactory_NewCoder_Askを呼び出す(t *testing.T) {
	t.Parallel()
	stub := &stubLLM{response: "```go\npackage main\n```"}
	f := agent.NewFactory(stub, config.DefaultGoConfig())
	coder := f.NewCoder()

	resp, err := coder.Ask(context.Background(), "Implement Reverse")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "```go\npackage main\n```" {
		t.Errorf("unexpected response: %q", resp)
	}
}

func TestFactory_NewCoderAgent_GenerateCodeを呼び出す(t *testing.T) {
	t.Parallel()
	want := agent.CodeGenerationResult{
		Files: []agent.FilePatch{
			{Path: "task.go", Content: "package task\n"},
		},
	}
	stub := &stubLLM{structuredResponse: want}
	f := agent.NewFactory(stub, config.DefaultGoConfig())
	coder := f.NewCoderAgent()

	got, err := coder.GenerateCode(context.Background(), "Implement Reverse")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Files) != 1 || got.Files[0].Path != "task.go" {
		t.Errorf("unexpected result: %+v", got)
	}
	if !strings.Contains(stub.lastPrompt, "Implement Reverse") {
		t.Errorf("task not forwarded to LLM; prompt=%q", stub.lastPrompt)
	}
}

func TestFactory_NewCoderAgent_LLMエラー時にエラーを返す(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("structured llm error")
	stub := &stubLLM{structuredErr: wantErr}
	f := agent.NewFactory(stub, config.DefaultGoConfig())
	coder := f.NewCoderAgent()

	_, err := coder.GenerateCode(context.Background(), "task")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap wantErr; got: %v", err)
	}
}

func TestFactory_NewReviewer_Askを呼び出す(t *testing.T) {
	t.Parallel()
	stub := &stubLLM{response: "Approve"}
	f := agent.NewFactory(stub, config.DefaultGoConfig())
	reviewer := f.NewReviewer()

	resp, err := reviewer.Ask(context.Background(), "diff content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "Approve" {
		t.Errorf("response=%q; want Approve", resp)
	}
}

func TestAgent_Ask_LLMエラー時にエラーを返す(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("llm unavailable")
	stub := &stubLLM{err: wantErr}
	f := agent.NewFactory(stub, config.DefaultGoConfig())
	planner := f.NewPlannerAgent()

	_, err := planner.Ask(context.Background(), "task")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap wantErr; got: %v", err)
	}
}

func TestAgent_Ask_プロンプトにシステムプロンプトとタスクが含まれる(t *testing.T) {
	t.Parallel()
	stub := &stubLLM{response: "ok"}
	f := agent.NewFactory(stub, config.DefaultGoConfig())
	planner := f.NewPlannerAgent()

	task := "unique-task-string-12345"
	if _, err := planner.Ask(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify that the separator and task are both present in the forwarded prompt.
	if !strings.Contains(stub.lastPrompt, "---") {
		t.Errorf("separator '---' not found in prompt: %q", stub.lastPrompt)
	}
	if !strings.Contains(stub.lastPrompt, task) {
		t.Errorf("task not found in prompt: %q", stub.lastPrompt)
	}
}
