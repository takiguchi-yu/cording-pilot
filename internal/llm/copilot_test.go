package llm_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/takiguchi-yu/cording-pilot/internal/llm"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

// ── NewCopilotClient ──────────────────────────────────────────────────────────

func TestNewCopilotClient_tokenが空の場合はエラーを返す(t *testing.T) {
	t.Parallel()
	log := logger.New(&strings.Builder{})
	_, err := llm.NewCopilotClient("gpt-4o", "", log)
	if err == nil {
		t.Fatal("空のトークンでエラーを期待しましたが nil でした")
	}
}

func TestNewCopilotClient_有効なトークンで生成に成功する(t *testing.T) {
	t.Parallel()
	log := logger.New(&strings.Builder{})
	c, err := llm.NewCopilotClient("gpt-4o", "dummy-token", log)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if c == nil {
		t.Fatal("nil CopilotClient が返されました")
	}
}

func TestNewCopilotClient_空のmodelはデフォルトモデルを使用する(t *testing.T) {
	t.Parallel()
	log := logger.New(&strings.Builder{})
	// model="" でもエラーにならないことを確認（内部でデフォルト値が補完される）。
	c, err := llm.NewCopilotClient("", "dummy-token", log)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if c == nil {
		t.Fatal("nil CopilotClient が返されました")
	}
}

// ── jsonSchemaFromValue ───────────────────────────────────────────────────────

func TestJSONSchemaFromValue_文字列型はstringを返す(t *testing.T) {
	t.Parallel()
	schema := llm.ExportJSONSchemaFromValue("")
	assertSchemaType(t, schema, "string")
}

func TestJSONSchemaFromValue_bool型はbooleanを返す(t *testing.T) {
	t.Parallel()
	schema := llm.ExportJSONSchemaFromValue(false)
	assertSchemaType(t, schema, "boolean")
}

func TestJSONSchemaFromValue_int型はintegerを返す(t *testing.T) {
	t.Parallel()
	schema := llm.ExportJSONSchemaFromValue(0)
	assertSchemaType(t, schema, "integer")
}

func TestJSONSchemaFromValue_float64型はnumberを返す(t *testing.T) {
	t.Parallel()
	schema := llm.ExportJSONSchemaFromValue(0.0)
	assertSchemaType(t, schema, "number")
}

func TestJSONSchemaFromValue_スライス型はarrayを返す(t *testing.T) {
	t.Parallel()
	schema := llm.ExportJSONSchemaFromValue([]string{})
	assertSchemaType(t, schema, "array")
	if _, ok := schema["items"]; !ok {
		t.Error("array スキーマには items フィールドが必要です")
	}
}

// ── jsonSchemaFromType ────────────────────────────────────────────────────────

func TestJSONSchemaFromType_構造体はobjectを返す(t *testing.T) {
	t.Parallel()
	type inner struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	schema := llm.ExportJSONSchemaFromType(reflect.TypeOf(inner{}))
	assertSchemaType(t, schema, "object")

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties フィールドが存在しません")
	}
	if _, exists := props["name"]; !exists {
		t.Error("properties に 'name' フィールドが存在しません")
	}
	if _, exists := props["age"]; !exists {
		t.Error("properties に 'age' フィールドが存在しません")
	}

	req, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required フィールドが存在しません")
	}
	if !containsString(req, "name") || !containsString(req, "age") {
		t.Errorf("required=%v; want to contain 'name' and 'age'", req)
	}

	if v, ok := schema["additionalProperties"].(bool); !ok || v {
		t.Error("additionalProperties は false でなければなりません")
	}
}

func TestJSONSchemaFromType_ポインタ型は非ポインタとして解決される(t *testing.T) {
	t.Parallel()
	schema := llm.ExportJSONSchemaFromType(reflect.TypeOf((*string)(nil)))
	assertSchemaType(t, schema, "string")
}

func TestJSONSchemaFromType_ネストした構造体を再帰的に解決する(t *testing.T) {
	t.Parallel()
	type child struct {
		Value string `json:"value"`
	}
	type parent struct {
		Child child `json:"child"`
	}
	schema := llm.ExportJSONSchemaFromType(reflect.TypeOf(parent{}))
	assertSchemaType(t, schema, "object")

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties フィールドが存在しません")
	}
	childSchema, ok := props["child"].(map[string]interface{})
	if !ok {
		t.Fatal("child プロパティが存在しません")
	}
	assertSchemaType(t, childSchema, "object")
}

// ── helpers ───────────────────────────────────────────────────────────────────

func assertSchemaType(t *testing.T, schema map[string]interface{}, wantType string) {
	t.Helper()
	got, ok := schema["type"].(string)
	if !ok {
		t.Fatalf("schema に type フィールドが存在しません: %v", schema)
	}
	if got != wantType {
		t.Errorf("schema.type=%q; want %q", got, wantType)
	}
}

func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
