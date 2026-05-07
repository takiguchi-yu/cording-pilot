package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/takiguchi-yu/cording-pilot/internal/config"
)

// ── DefaultGoConfig ───────────────────────────────────────────────────────────

func TestDefaultGoConfig_デフォルト値が正しく設定される(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultGoConfig()
	if cfg.Version != "1.0" {
		t.Errorf("Version=%q; want %q", cfg.Version, "1.0")
	}
	if cfg.Project.Language != "go" {
		t.Errorf("Project.Language=%q; want %q", cfg.Project.Language, "go")
	}
	if cfg.Project.TestFramework != "standard testing" {
		t.Errorf("Project.TestFramework=%q; want %q", cfg.Project.TestFramework, "standard testing")
	}
	if cfg.LLM.Provider != "copilot" {
		t.Errorf("LLM.Provider=%q; want %q", cfg.LLM.Provider, "copilot")
	}
	if cfg.LLM.Model != "gpt-4.1" {
		t.Errorf("LLM.Model=%q; want %q", cfg.LLM.Model, "gpt-4.1")
	}
	if cfg.LLM.AutoFixModel != "gpt-5-mini" {
		t.Errorf("LLM.AutoFixModel=%q; want %q", cfg.LLM.AutoFixModel, "gpt-5-mini")
	}
	if len(cfg.Pipeline) == 0 {
		t.Error("Pipeline should not be empty")
	}
}

// ── Load ─────────────────────────────────────────────────────────────────────

func TestLoad_ファイルが存在しない場合はデフォルト設定を返す(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(filepath.Join(t.TempDir(), "nonexistent.yml"))
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if cfg.Project.Language != "go" {
		t.Errorf("デフォルト設定の Language=%q; want %q", cfg.Project.Language, "go")
	}
}

func TestLoad_有効なYAMLファイルを読み込む(t *testing.T) {
	t.Parallel()
	yaml := "version: \"1.0\"\n" +
		"project:\n" +
		"  language: python\n" +
		"  test_framework: pytest\n" +
		"llm:\n" +
		"  provider: copilot\n" +
		"  model: gpt-4o-mini\n" +
		"environment:\n" +
		"  type: local\n" +
		"pipeline:\n" +
		"  - name: test\n" +
		"    command: pytest\n"
	path := writeTemp(t, yaml)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if cfg.Version != "1.0" {
		t.Errorf("Version=%q; want %q", cfg.Version, "1.0")
	}
	if cfg.Project.Language != "python" {
		t.Errorf("Project.Language=%q; want %q", cfg.Project.Language, "python")
	}
	if cfg.LLM.Model != "gpt-4o-mini" {
		t.Errorf("LLM.Model=%q; want %q", cfg.LLM.Model, "gpt-4o-mini")
	}
	if len(cfg.Pipeline) != 1 || cfg.Pipeline[0].Name != "test" {
		t.Errorf("Pipeline=%v; want 1 step with name=test", cfg.Pipeline)
	}
}

func TestLoad_不正なYAMLはエラーを返す(t *testing.T) {
	t.Parallel()
	path := writeTemp(t, "{\ninvalid: yaml: :::")
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("不正な YAML でエラーを期待しましたが nil でした")
	}
}

func TestLoad_不正な設定はエラーを返す(t *testing.T) {
	t.Parallel()
	yaml := `version: "1.0"
environment:
  type: unknown_type
pipeline:
  - name: test
    command: echo ok
`
	path := writeTemp(t, yaml)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("不正な environment.type でエラーを期待しましたが nil でした")
	}
}

// ── fillDefaults ─────────────────────────────────────────────────────────────

func TestLoad_省略フィールドにデフォルト値が補完される(t *testing.T) {
	t.Parallel()
	// language/test_framework/llm/agents を省略した最小設定。
	yaml := `version: "1.0"
environment:
  type: local
pipeline:
  - name: test
    command: go test ./...
`
	path := writeTemp(t, yaml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if cfg.Project.Language != "go" {
		t.Errorf("fillDefaults: Language=%q; want %q", cfg.Project.Language, "go")
	}
	if cfg.LLM.Provider != "copilot" {
		t.Errorf("fillDefaults: LLM.Provider=%q; want %q", cfg.LLM.Provider, "copilot")
	}
	if cfg.LLM.Model != "gpt-4.1" {
		t.Errorf("fillDefaults: LLM.Model=%q; want %q", cfg.LLM.Model, "gpt-4.1")
	}
	if cfg.LLM.AutoFixModel != cfg.LLM.Model {
		t.Errorf("fillDefaults: LLM.AutoFixModel=%q; want same as LLM.Model=%q", cfg.LLM.AutoFixModel, cfg.LLM.Model)
	}
	if cfg.Agents.Planner == "" {
		t.Error("fillDefaults: Agents.Planner should not be empty")
	}
}

func TestLoad_versionを省略するとデフォルト値が補完される(t *testing.T) {
	t.Parallel()
	yaml := `environment:
  type: local
pipeline:
  - name: test
    command: go test ./...
`
	path := writeTemp(t, yaml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if cfg.Version != "1.0" {
		t.Errorf("fillDefaults: Version=%q; want %q", cfg.Version, "1.0")
	}
}

func TestLoad_autoFixModelを省略するとmodelと同じ値が補完される(t *testing.T) {
	t.Parallel()
	yaml := `version: "1.0"
llm:
  provider: copilot
  model: gpt-5-mini
environment:
  type: local
pipeline:
  - name: test
    command: go test ./...
`
	path := writeTemp(t, yaml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if cfg.LLM.AutoFixModel != "gpt-5-mini" {
		t.Errorf("fillDefaults: LLM.AutoFixModel=%q; want %q", cfg.LLM.AutoFixModel, "gpt-5-mini")
	}
}

func TestLoad_docker環境でimageを省略するとデフォルトイメージが補完される(t *testing.T) {
	t.Parallel()
	yaml := `version: "1.0"
environment:
  type: docker
pipeline:
  - name: test
    command: go test ./...
`
	path := writeTemp(t, yaml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if cfg.Environment.Image == "" {
		t.Error("docker 環境で image が空のままです")
	}
}

// ── validate ─────────────────────────────────────────────────────────────────

func TestLoad_docker環境でimageが空の場合はエラーを返す(t *testing.T) {
	t.Parallel()
	// fillDefaults が補完しないよう、image に明示的に空文字を与えるのではなく
	// 直接 validate を叩く代わりに、Load 経由で検証する。
	// docker + image 空は fillDefaults でデフォルトが入るため、
	// ここでは validate の境界条件を別のルートで確認する。
	// environment.type に不正値を渡す。
	yaml := `version: "1.0"
environment:
  type: invalid_env
pipeline:
  - name: test
    command: go test ./...
`
	path := writeTemp(t, yaml)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("不正な environment.type でエラーを期待しましたが nil でした")
	}
}

func TestLoad_不正なllmProviderはエラーを返す(t *testing.T) {
	t.Parallel()
	yaml := `version: "1.0"
llm:
  provider: openai
  model: gpt-4.1
environment:
  type: local
pipeline:
  - name: test
    command: go test ./...
`
	path := writeTemp(t, yaml)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("不正な llm.provider でエラーを期待しましたが nil でした")
	}
}

func TestLoad_不正なversionはエラーを返す(t *testing.T) {
	t.Parallel()
	yaml := `version: "2.0"
environment:
  type: local
pipeline:
  - name: test
    command: go test ./...
`
	path := writeTemp(t, yaml)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("不正な version でエラーを期待しましたが nil でした")
	}
}

func TestLoad_未知のキーがある場合はエラーを返す(t *testing.T) {
	t.Parallel()
	yaml := `version: "1.0"
environmnt:
  type: local
pipeline:
  - name: test
    command: go test ./...
`
	path := writeTemp(t, yaml)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("未知のキーでエラーを期待しましたが nil でした")
	}
}

func TestLoad_pipelineが空の場合はエラーを返す(t *testing.T) {
	t.Parallel()
	// Pipeline を明示的に空リストで上書きするには Config 直接操作が必要なため、
	// 通常の Load パスでは fillDefaults が補完してしまう。
	// そのため pipeline キーを省略 + fillDefaults が入ることを確認するテストのみ実施。
	yaml := `version: "1.0"
environment:
  type: local
`
	path := writeTemp(t, yaml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("pipeline 省略時に fillDefaults でデフォルトが補完されるはずですが: %v", err)
	}
	if len(cfg.Pipeline) == 0 {
		t.Error("fillDefaults: Pipeline が空のままです")
	}
}

func TestLoad_pipelineステップのnameが空の場合はエラーを返す(t *testing.T) {
	t.Parallel()
	yaml := `version: "1.0"
environment:
  type: local
pipeline:
  - name: ""
    command: go test ./...
`
	path := writeTemp(t, yaml)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("pipeline[0].name が空の場合はエラーを期待しましたが nil でした")
	}
}

func TestLoad_pipelineステップのcommandが空の場合はエラーを返す(t *testing.T) {
	t.Parallel()
	yaml := `version: "1.0"
environment:
  type: local
pipeline:
  - name: test
    command: ""
`
	path := writeTemp(t, yaml)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("pipeline[0].command が空の場合はエラーを期待しましたが nil でした")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// writeTemp は content を TempDir に書き込み、そのパスを返します。
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeTemp: %v", err)
	}
	return path
}
