// Package llm の CopilotClient は GitHub Copilot (GitHub Models API) を使用する Client 実装です。
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
	"github.com/takiguchi-yu/cording-pilot/pkg/retry"
)

const (
	copilotBaseURL         = "https://models.inference.ai.azure.com"
	rateLimitModeFailFast  = "fail_fast"
	rateLimitModeHonorWait = "honor_wait"
)

var waitSecondsPattern = regexp.MustCompile(`Please wait\s+(\d+)\s+seconds`)

// CopilotOptions は CopilotClient のリトライ・レートリミット挙動を制御します。
type CopilotOptions struct {
	RetryPolicy      retry.Policy
	RateLimitMode    string
	MaxRateLimitWait time.Duration
}

// CopilotClient は GitHub Copilot (GitHub Models API) を使用する llm.Client 実装です。
// ゴルーチン安全です。
type CopilotClient struct {
	client           *openai.Client
	model            string
	log              *logger.Logger
	retryPolicy      retry.Policy
	rateLimitMode    string
	maxRateLimitWait time.Duration
}

// NewCopilotClient は指定したトークンとモデルで CopilotClient を生成します。
// token が空の場合はエラーを返します。
func NewCopilotClient(model, token string, log *logger.Logger) (*CopilotClient, error) {
	return NewCopilotClientWithOptions(model, token, log, CopilotOptions{})
}

// NewCopilotClientWithOptions は指定したトークン・モデル・挙動オプションで CopilotClient を生成します。
func NewCopilotClientWithOptions(model, token string, log *logger.Logger, opts CopilotOptions) (*CopilotClient, error) {
	if token == "" {
		return nil, fmt.Errorf("llm: GITHUB_TOKEN 環境変数が設定されていません")
	}
	if model == "" {
		model = "gpt-4o"
	}
	retryPolicy := opts.RetryPolicy
	if retryPolicy.MaxAttempts == 0 {
		retryPolicy = retry.DefaultPolicy
	}
	rateLimitMode := strings.ToLower(strings.TrimSpace(opts.RateLimitMode))
	if rateLimitMode == "" {
		rateLimitMode = rateLimitModeFailFast
	}
	maxRateLimitWait := opts.MaxRateLimitWait
	if maxRateLimitWait <= 0 {
		maxRateLimitWait = 30 * time.Second
	}
	config := openai.DefaultConfig(token)
	config.BaseURL = copilotBaseURL
	return &CopilotClient{
		client:           openai.NewClientWithConfig(config),
		model:            model,
		log:              log,
		retryPolicy:      retryPolicy,
		rateLimitMode:    rateLimitMode,
		maxRateLimitWait: maxRateLimitWait,
	}, nil
}

// Generate implements Client.
// プロンプトを GitHub Models API に送信し、生成されたテキストを返します。
// レートリミットやネットワークエラー発生時は pkg/retry の DefaultPolicy に従いリトライします。
func (c *CopilotClient) Generate(ctx context.Context, prompt string) (string, error) {
	_ = c.log.Debug("llm.generate.request", fmt.Sprintf("prompt=%q", prompt))

	var result string
	err := retry.Do(ctx, c.retryPolicy, func() error {
		resp, apiErr := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model: c.model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleUser, Content: prompt},
			},
		})
		if apiErr != nil {
			return c.handleAPIError(ctx, "llm: generate", apiErr)
		}
		result = resp.Choices[0].Message.Content
		return nil
	})
	if err != nil {
		return "", err
	}

	_ = c.log.Debug("llm.generate.response", fmt.Sprintf("response=%q", result))
	return result, nil
}

// GenerateStructured implements Client.
// プロンプトを GitHub Models API に送信し、構造化 JSON を取得して target にデコードします。
// モデル名に "claude" が含まれる場合はプロンプトに JSON スキーマを埋め込む方式を使用し、
// それ以外のモデルでは ResponseFormatJSONSchema を使用します。
// JSON デコードに失敗した場合は ErrJSONParse をラップしたエラーを返します。
func (c *CopilotClient) GenerateStructured(ctx context.Context, prompt string, target interface{}) error {
	schema := jsonSchemaFromValue(target)
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("llm: marshal schema: %w", err)
	}

	isClaude := strings.Contains(c.model, "claude")

	userPrompt := prompt
	if isClaude {
		userPrompt = prompt + "\n\n【重要】以下の JSON スキーマに完全に一致する JSON 文字列のみを出力してください。Markdown ブロック（```json 等）や前後の説明は一切含めないでください。\n" + string(schemaBytes)
	}

	_ = c.log.Debug("llm.generateStructured.request", fmt.Sprintf("prompt=%q schema=%s isClaude=%v", userPrompt, schemaBytes, isClaude))

	var raw string
	retryErr := retry.Do(ctx, c.retryPolicy, func() error {
		req := openai.ChatCompletionRequest{
			Model: c.model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleUser, Content: userPrompt},
			},
		}
		if !isClaude {
			req.ResponseFormat = &openai.ChatCompletionResponseFormat{
				Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
				JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
					Name:   "output",
					Schema: jsonMarshalMap(schema),
					Strict: true,
				},
			}
		}
		resp, apiErr := c.client.CreateChatCompletion(ctx, req)
		if apiErr != nil {
			return c.handleAPIError(ctx, "llm: generate structured", apiErr)
		}
		raw = resp.Choices[0].Message.Content
		return nil
	})
	if retryErr != nil {
		return retryErr
	}

	_ = c.log.Debug("llm.generateStructured.response", fmt.Sprintf("raw=%q", raw))

	// Claude がMarkdownコードブロックを付けて返してくる場合に備えてサニタイズする。
	sanitized := sanitizeJSONResponse(raw)

	if jsonErr := json.Unmarshal([]byte(sanitized), target); jsonErr != nil {
		_ = c.log.Error("llm.generateStructured.parseError", fmt.Sprintf("error=%v raw=%q", jsonErr, raw))
		return fmt.Errorf("%w: %w (raw: %s)", ErrJSONParse, jsonErr, raw)
	}
	return nil
}

// sanitizeJSONResponse は LLM レスポンス文字列の前後に付与された
// Markdown コードブロック（```json ... ``` 等）を除去し、純粋な JSON 文字列を返します。
func sanitizeJSONResponse(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// 最初の行（```json 等のフェンス行）を除去する。
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
		// 末尾の ``` を除去する。
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	return s
}

func (c *CopilotClient) handleAPIError(ctx context.Context, operation string, apiErr error) error {
	var openAIErr *openai.APIError
	if !errors.As(apiErr, &openAIErr) {
		return fmt.Errorf("%s: %w", operation, apiErr)
	}

	if openAIErr.HTTPStatusCode != 429 {
		return fmt.Errorf("%s: %w", operation, apiErr)
	}

	message := strings.TrimSpace(openAIErr.Message)
	waitSeconds, hasWaitSeconds := extractWaitSeconds(message)
	_ = c.log.Warn("llm.rate_limit", fmt.Sprintf("model=%s mode=%s wait_seconds=%d message=%q", c.model, c.rateLimitMode, waitSeconds, message))

	if isDailyLimitMessage(message) || c.rateLimitMode == rateLimitModeFailFast {
		return retry.NonRetryable(fmt.Errorf("%s: %w", operation, apiErr))
	}

	if c.rateLimitMode != rateLimitModeHonorWait || !hasWaitSeconds {
		return fmt.Errorf("%s: %w", operation, apiErr)
	}

	waitDuration := time.Duration(waitSeconds) * time.Second
	if waitDuration > c.maxRateLimitWait {
		return retry.NonRetryable(fmt.Errorf("%s: rate limit wait %s exceeds max wait %s: %w", operation, waitDuration, c.maxRateLimitWait, apiErr))
	}

	if err := c.waitForRateLimit(ctx, waitDuration); err != nil {
		return retry.NonRetryable(fmt.Errorf("%s: wait for rate limit reset: %w", operation, err))
	}

	return fmt.Errorf("%s: %w", operation, apiErr)
}

func (c *CopilotClient) waitForRateLimit(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}

	_ = c.log.Info("llm.rate_limit_wait", fmt.Sprintf("model=%s waiting=%s", c.model, d))

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func extractWaitSeconds(message string) (int, bool) {
	matches := waitSecondsPattern.FindStringSubmatch(message)
	if len(matches) != 2 {
		return 0, false
	}
	seconds, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, false
	}
	if seconds < 0 {
		return 0, false
	}
	return seconds, true
}

func isDailyLimitMessage(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "userbymodelbyday") || strings.Contains(lower, "per 86400s exceeded")
}

// jsonMarshalMap は json.Marshaler を実装する map ラッパーです。
// openai.ChatCompletionResponseFormatJSONSchema の Schema フィールドに渡すために使用します。
type jsonMarshalMap map[string]interface{}

// MarshalJSON implements json.Marshaler.
func (m jsonMarshalMap) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(m))
}

// jsonSchemaFromValue は target の型情報から JSON Schema を表す map を生成します。
func jsonSchemaFromValue(v interface{}) map[string]interface{} {
	return jsonSchemaFromType(reflect.TypeOf(v))
}

// jsonSchemaFromType は reflect.Type から再帰的に JSON Schema を構築します。
// OpenAI Structured Outputs の strict モードに対応するため、
// すべてのオブジェクトに additionalProperties: false を付与します。
func jsonSchemaFromType(t reflect.Type) map[string]interface{} {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		props := map[string]interface{}{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := f.Tag.Get("json")
			parts := strings.Split(tag, ",")
			name := parts[0]
			if name == "" || name == "-" {
				name = f.Name
			}
			props[name] = jsonSchemaFromType(f.Type)
			if !hasJSONOption(parts[1:], "omitempty") {
				required = append(required, name)
			}
		}
		return map[string]interface{}{
			"type":                 "object",
			"properties":           props,
			"required":             required,
			"additionalProperties": false,
		}
	case reflect.Slice:
		return map[string]interface{}{
			"type":  "array",
			"items": jsonSchemaFromType(t.Elem()),
		}
	case reflect.String:
		return map[string]interface{}{"type": "string"}
	case reflect.Bool:
		return map[string]interface{}{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]interface{}{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]interface{}{"type": "number"}
	default:
		return map[string]interface{}{"type": "string"}
	}
}

func hasJSONOption(options []string, target string) bool {
	for _, opt := range options {
		if strings.TrimSpace(opt) == target {
			return true
		}
	}
	return false
}
