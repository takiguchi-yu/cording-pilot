package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	openai "github.com/sashabaranov/go-openai"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

const (
	// DefaultOllamaBaseURL は Ollama の OpenAI 互換 API の既定エンドポイントです。
	DefaultOllamaBaseURL = "http://localhost:11434/v1"
	ollamaDummyAPIKey    = "ollama-dummy-key"
	defaultOllamaModel   = "llama3.1:8b"
	maxOllamaPromptChars = 4000
)

// OllamaClient は Ollama の OpenAI 互換 API を利用する llm.Client 実装です。
// ゴルーチン安全です。
type OllamaClient struct {
	client *openai.Client
	model  string
	log    *logger.Logger
}

// NewOllamaClient は指定したモデルと BaseURL で OllamaClient を生成します。
// baseURL が空の場合は DefaultOllamaBaseURL を使用します。
func NewOllamaClient(model, baseURL string, log *logger.Logger) (*OllamaClient, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		model = defaultOllamaModel
	}
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = DefaultOllamaBaseURL
	}

	config := openai.DefaultConfig(ollamaDummyAPIKey)
	config.BaseURL = baseURL

	return &OllamaClient{
		client: openai.NewClientWithConfig(config),
		model:  model,
		log:    log,
	}, nil
}

// Generate implements Client.
func (c *OllamaClient) Generate(ctx context.Context, prompt string) (string, error) {
	limitedPrompt := truncateForOllamaContext(prompt, maxOllamaPromptChars)
	_ = c.log.Debug("llm.ollama.generate.request", fmt.Sprintf("prompt=%q", limitedPrompt))

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: limitedPrompt},
		},
	})
	if err != nil {
		return "", wrapOllamaAPIError("llm: ollama generate", err)
	}

	result := resp.Choices[0].Message.Content
	_ = c.log.Debug("llm.ollama.generate.response", fmt.Sprintf("response=%q", result))
	return result, nil
}

// GenerateStructured implements Client.
// Ollama では Strict JSON Schema モードを使わず、json_object + プロンプト指示で構造化出力を強制します。
func (c *OllamaClient) GenerateStructured(ctx context.Context, prompt string, target interface{}) error {
	limitedPrompt := truncateForOllamaContext(prompt, maxOllamaPromptChars)

	schema := jsonSchemaFromValue(target)
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("llm: marshal schema: %w", err)
	}

	schemaInstruction := "【重要】以下の JSON スキーマに厳密に従う JSON 文字列のみを出力してください。" +
		" Markdown コードブロック（```json 等）や説明文は含めないでください。\n" + string(schemaBytes)

	_ = c.log.Debug("llm.ollama.generateStructured.request", fmt.Sprintf("prompt=%q schema=%s", limitedPrompt, schemaBytes))

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: schemaInstruction},
			{Role: openai.ChatMessageRoleUser, Content: limitedPrompt},
		},
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		return wrapOllamaAPIError("llm: ollama generate structured", err)
	}

	raw := resp.Choices[0].Message.Content
	_ = c.log.Debug("llm.ollama.generateStructured.response", fmt.Sprintf("raw=%q", raw))

	if jsonErr := decodeStructuredJSON(raw, target); jsonErr != nil {
		_ = c.log.Warn("llm.ollama.generateStructured.repair.start", fmt.Sprintf("initial_error=%v", jsonErr))
		repairedRaw, repairErr := c.repairStructuredJSON(ctx, raw, string(schemaBytes))
		if repairErr != nil {
			_ = c.log.Error("llm.ollama.generateStructured.parseError", fmt.Sprintf("error=%v raw=%q", jsonErr, raw))
			return fmt.Errorf("%w: %w (raw: %s)", ErrJSONParse, jsonErr, raw)
		}
		if repairDecodeErr := decodeStructuredJSON(repairedRaw, target); repairDecodeErr != nil {
			_ = c.log.Error("llm.ollama.generateStructured.parseError", fmt.Sprintf("error=%v raw=%q repaired_raw=%q", repairDecodeErr, raw, repairedRaw))
			return fmt.Errorf("%w: %w (raw: %s)", ErrJSONParse, repairDecodeErr, repairedRaw)
		}
		_ = c.log.Info("llm.ollama.generateStructured.repair.success", "壊れた JSON を修復してデコードしました")
	}
	return nil
}

func (c *OllamaClient) repairStructuredJSON(ctx context.Context, raw string, schema string) (string, error) {
	repairSource := truncateForOllamaContext(raw, maxOllamaPromptChars)
	repairPrompt := "以下は壊れた JSON です。JSON を修復し、必ず JSON のみを返してください。説明文や Markdown は禁止です。\n\n" +
		"## 必須スキーマ\n" + schema + "\n\n" +
		"## 壊れた JSON\n" + repairSource

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "あなたは JSON 修復エンジンです。壊れた JSON をスキーマに沿って修復し、有効な JSON のみを返します。"},
			{Role: openai.ChatMessageRoleUser, Content: repairPrompt},
		},
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		return "", wrapOllamaAPIError("llm: ollama repair structured", err)
	}

	return resp.Choices[0].Message.Content, nil
}

func decodeStructuredJSON(raw string, target interface{}) error {
	sanitized := sanitizeJSONResponse(raw)
	if err := json.Unmarshal([]byte(sanitized), target); err == nil {
		return nil
	}

	var nestedJSON string
	if err := json.Unmarshal([]byte(sanitized), &nestedJSON); err == nil {
		nestedSanitized := sanitizeJSONResponse(nestedJSON)
		if nestedErr := json.Unmarshal([]byte(nestedSanitized), target); nestedErr == nil {
			return nil
		}
	}

	if obj, ok := extractFirstJSONObject(sanitized); ok {
		if err := json.Unmarshal([]byte(obj), target); err == nil {
			return nil
		}
	}

	return json.Unmarshal([]byte(sanitized), target)
}

func extractFirstJSONObject(s string) (string, bool) {
	start := strings.IndexByte(s, '{')
	if start == -1 {
		return "", false
	}

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}

	return "", false
}

func wrapOllamaAPIError(operation string, err error) error {
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		return fmt.Errorf("%s: %w", operation, err)
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("%s: Ollamaサーバーに接続できません。'make ollama-serve' を実行しているか確認してください: %w", operation, err)
	}

	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "connection refused") || strings.Contains(lower, "connect:") {
		return fmt.Errorf("%s: Ollamaサーバーに接続できません。'make ollama-serve' を実行しているか確認してください: %w", operation, err)
	}

	return fmt.Errorf("%s: %w", operation, err)
}

func truncateForOllamaContext(prompt string, maxChars int) string {
	if maxChars <= 0 {
		return prompt
	}

	runeCount := utf8.RuneCountInString(prompt)
	if runeCount <= maxChars {
		return prompt
	}

	const marker = "\n\n[truncated for ollama context guard]\n\n"
	headChars := maxChars * 3 / 4
	if headChars < 1 {
		headChars = 1
	}
	tailChars := maxChars - headChars
	if tailChars < 1 {
		tailChars = 1
	}

	runes := []rune(prompt)
	if len(runes) <= headChars+tailChars {
		return string(runes[:maxChars])
	}

	return string(runes[:headChars]) + marker + string(runes[len(runes)-tailChars:])
}
