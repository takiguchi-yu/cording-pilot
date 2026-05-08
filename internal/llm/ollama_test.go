package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/takiguchi-yu/cording-pilot/internal/llm"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

func TestNewOllamaClient_空のbaseURLはデフォルトを使う(t *testing.T) {
	t.Parallel()
	log := logger.New(&strings.Builder{})
	c, err := llm.NewOllamaClient("llama3.1:8b", "", log)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if c == nil {
		t.Fatal("nil OllamaClient が返されました")
	}
}

func TestOllamaClient_GenerateStructured_jsonObjectで要求しレスポンスをデコードする(t *testing.T) {
	t.Parallel()

	type requestBody struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		ResponseFormat struct {
			Type string `json:"type"`
		} `json:"response_format"`
	}

	var captured requestBody
	var capturedPath string
	var decodeErr error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		defer func() { _ = r.Body.Close() }()
		decodeErr = json.NewDecoder(r.Body).Decode(&captured)
		resp := map[string]any{
			"id":      "chatcmpl-1",
			"object":  "chat.completion",
			"created": 0,
			"model":   "llama3.1:8b",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "```json\n{\"files\":[{\"path\":\"a.txt\"}]}\n```",
					},
					"finish_reason": "stop",
				},
			},
		}
		payload, err := json.Marshal(resp)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	log := logger.New(&strings.Builder{})
	client, err := llm.NewOllamaClient("llama3.1:8b", server.URL+"/v1", log)
	if err != nil {
		t.Fatalf("client create error: %v", err)
	}

	var out struct {
		Files []struct {
			Path string `json:"path"`
		} `json:"files"`
	}
	if err := client.GenerateStructured(context.Background(), "[TEST_GEN] generate files", &out); err != nil {
		t.Fatalf("GenerateStructured error: %v", err)
	}
	if decodeErr != nil {
		t.Fatalf("request decode error: %v", decodeErr)
	}
	if capturedPath != "/v1/chat/completions" {
		t.Fatalf("unexpected path: %s", capturedPath)
	}

	if captured.ResponseFormat.Type != "json_object" {
		t.Fatalf("response_format.type=%q; want json_object", captured.ResponseFormat.Type)
	}
	if len(captured.Messages) < 2 {
		t.Fatalf("messages len=%d; want >=2", len(captured.Messages))
	}
	if !strings.Contains(captured.Messages[0].Content, "JSON スキーマ") {
		t.Fatalf("system message should contain schema instruction: %q", captured.Messages[0].Content)
	}
	if len(out.Files) != 1 || out.Files[0].Path != "a.txt" {
		t.Fatalf("unexpected decoded output: %+v", out)
	}
}

func TestOllamaClient_Generate_接続不能時は分かりやすいエラーを返す(t *testing.T) {
	t.Parallel()
	log := logger.New(&strings.Builder{})
	client, err := llm.NewOllamaClient("llama3.1:8b", "http://127.0.0.1:1/v1", log)
	if err != nil {
		t.Fatalf("client create error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = client.Generate(ctx, "hello")
	if err == nil {
		t.Fatal("接続不能エラーを期待しましたが nil でした")
	}
	if !strings.Contains(err.Error(), "Ollamaサーバーに接続できません。'make ollama-serve' を実行しているか確認してください") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestOllamaClient_Generate_長大プロンプトはコンテキスト保護のため切り詰める(t *testing.T) {
	t.Parallel()

	type requestBody struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	var captured requestBody
	var decodeErr error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		decodeErr = json.NewDecoder(r.Body).Decode(&captured)
		resp := map[string]any{
			"id":      "chatcmpl-1",
			"object":  "chat.completion",
			"created": 0,
			"model":   "llama3.1:8b",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "ok",
				},
				"finish_reason": "stop",
			}},
		}
		payload, err := json.Marshal(resp)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	log := logger.New(&strings.Builder{})
	client, err := llm.NewOllamaClient("llama3.1:8b", server.URL+"/v1", log)
	if err != nil {
		t.Fatalf("client create error: %v", err)
	}

	longPrompt := strings.Repeat("a", 5000)
	_, err = client.Generate(context.Background(), longPrompt)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	if decodeErr != nil {
		t.Fatalf("request decode error: %v", decodeErr)
	}
	if len(captured.Messages) != 1 {
		t.Fatalf("messages len=%d; want 1", len(captured.Messages))
	}
	if !strings.Contains(captured.Messages[0].Content, "[truncated for ollama context guard]") {
		t.Fatalf("prompt should contain truncation marker")
	}
	if len([]rune(captured.Messages[0].Content)) >= len([]rune(longPrompt)) {
		t.Fatalf("prompt should be truncated")
	}
}

func TestOllamaClient_GenerateStructured_長大プロンプトはコンテキスト保護のため切り詰める(t *testing.T) {
	t.Parallel()

	type requestBody struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	var captured requestBody
	var decodeErr error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		decodeErr = json.NewDecoder(r.Body).Decode(&captured)
		resp := map[string]any{
			"id":      "chatcmpl-1",
			"object":  "chat.completion",
			"created": 0,
			"model":   "llama3.1:8b",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "{\"files\":[]}",
				},
				"finish_reason": "stop",
			}},
		}
		payload, err := json.Marshal(resp)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	log := logger.New(&strings.Builder{})
	client, err := llm.NewOllamaClient("llama3.1:8b", server.URL+"/v1", log)
	if err != nil {
		t.Fatalf("client create error: %v", err)
	}

	var out struct {
		Files []struct {
			Path string `json:"path"`
		} `json:"files"`
	}
	longPrompt := strings.Repeat("b", 5000)
	if err := client.GenerateStructured(context.Background(), longPrompt, &out); err != nil {
		t.Fatalf("GenerateStructured error: %v", err)
	}
	if decodeErr != nil {
		t.Fatalf("request decode error: %v", decodeErr)
	}
	if len(captured.Messages) < 2 {
		t.Fatalf("messages len=%d; want >=2", len(captured.Messages))
	}
	if !strings.Contains(captured.Messages[1].Content, "[truncated for ollama context guard]") {
		t.Fatalf("user prompt should contain truncation marker")
	}
	if len([]rune(captured.Messages[1].Content)) >= len([]rune(longPrompt)) {
		t.Fatalf("prompt should be truncated")
	}
}
