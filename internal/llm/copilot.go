// Package llm の CopilotClient は GitHub Copilot (GitHub Models API) を使用する Client 実装です。
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	openai "github.com/sashabaranov/go-openai"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
	"github.com/takiguchi-yu/cording-pilot/pkg/retry"
)

const (
	copilotBaseURL = "https://models.inference.ai.azure.com"
)

// CopilotClient は GitHub Copilot (GitHub Models API) を使用する llm.Client 実装です。
// ゴルーチン安全です。
type CopilotClient struct {
	client *openai.Client
	model  string
	log    *logger.Logger
}

// NewCopilotClient は指定したトークンとモデルで CopilotClient を生成します。
// token が空の場合はエラーを返します。
func NewCopilotClient(model, token string, log *logger.Logger) (*CopilotClient, error) {
	if token == "" {
		return nil, fmt.Errorf("llm: GITHUB_TOKEN 環境変数が設定されていません")
	}
	if model == "" {
		model = "gpt-4o"
	}
	config := openai.DefaultConfig(token)
	config.BaseURL = copilotBaseURL
	return &CopilotClient{
		client: openai.NewClientWithConfig(config),
		model:  model,
		log:    log,
	}, nil
}

// Generate implements Client.
// プロンプトを GitHub Models API に送信し、生成されたテキストを返します。
// レートリミットやネットワークエラー発生時は pkg/retry の DefaultPolicy に従いリトライします。
func (c *CopilotClient) Generate(ctx context.Context, prompt string) (string, error) {
	_ = c.log.Debug("llm.generate.request", fmt.Sprintf("prompt=%q", prompt))

	var result string
	err := retry.Do(ctx, retry.DefaultPolicy, func() error {
		resp, apiErr := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model: c.model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleUser, Content: prompt},
			},
		})
		if apiErr != nil {
			return fmt.Errorf("llm: generate: %w", apiErr)
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
// プロンプトを GitHub Models API に送信し、ResponseFormatJSONSchema を使用して
// 構造化 JSON を取得し target にデコードします。
// JSON デコードに失敗した場合は ErrJSONParse をラップしたエラーを返します。
func (c *CopilotClient) GenerateStructured(ctx context.Context, prompt string, target interface{}) error {
	schema := jsonSchemaFromValue(target)
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("llm: marshal schema: %w", err)
	}

	_ = c.log.Debug("llm.generateStructured.request", fmt.Sprintf("prompt=%q schema=%s", prompt, schemaBytes))

	var raw string
	retryErr := retry.Do(ctx, retry.DefaultPolicy, func() error {
		resp, apiErr := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model: c.model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleUser, Content: prompt},
			},
			ResponseFormat: &openai.ChatCompletionResponseFormat{
				Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
				JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
					Name:   "output",
					Schema: jsonMarshalMap(schema),
					Strict: true,
				},
			},
		})
		if apiErr != nil {
			return fmt.Errorf("llm: generate structured: %w", apiErr)
		}
		raw = resp.Choices[0].Message.Content
		return nil
	})
	if retryErr != nil {
		return retryErr
	}

	_ = c.log.Debug("llm.generateStructured.response", fmt.Sprintf("raw=%q", raw))

	if jsonErr := json.Unmarshal([]byte(raw), target); jsonErr != nil {
		_ = c.log.Error("llm.generateStructured.parseError", fmt.Sprintf("error=%v raw=%q", jsonErr, raw))
		return fmt.Errorf("%w: %w", ErrJSONParse, jsonErr)
	}
	return nil
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
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		props := map[string]interface{}{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := f.Tag.Get("json")
			name := strings.Split(tag, ",")[0]
			if name == "" || name == "-" {
				name = f.Name
			}
			props[name] = jsonSchemaFromType(f.Type)
			required = append(required, name)
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
