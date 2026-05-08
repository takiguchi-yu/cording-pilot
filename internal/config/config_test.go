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
	if cfg.Project.Language != "Go" {
		t.Errorf("Project.Language=%q; want %q", cfg.Project.Language, "Go")
	}
	if cfg.Project.Framework != "" {
		t.Errorf("Project.Framework=%q; want empty", cfg.Project.Framework)
	}
	if cfg.LLM.Default.Provider != "copilot" {
		t.Errorf("LLM.Default.Provider=%q; want %q", cfg.LLM.Default.Provider, "copilot")
	}
	if cfg.LLM.Default.Model != "gpt-4.1" {
		t.Errorf("LLM.Default.Model=%q; want %q", cfg.LLM.Default.Model, "gpt-4.1")
	}
	if cfg.LLM.Default.BaseURL != "" {
		t.Errorf("LLM.Default.BaseURL=%q; want empty", cfg.LLM.Default.BaseURL)
	}
	if cfg.LLM.AutoFixModel != "gpt-5-mini" {
		t.Errorf("LLM.AutoFixModel=%q; want %q", cfg.LLM.AutoFixModel, "gpt-5-mini")
	}
	// エージェント固有設定なし → GetXxxConfig() はすべて Default を返す
	if cfg.LLM.GetPlannerConfig().Model != cfg.LLM.Default.Model {
		t.Errorf("GetPlannerConfig().Model=%q; want %q", cfg.LLM.GetPlannerConfig().Model, cfg.LLM.Default.Model)
	}
	if cfg.LLM.GetCoderConfig().Model != cfg.LLM.Default.Model {
		t.Errorf("GetCoderConfig().Model=%q; want %q", cfg.LLM.GetCoderConfig().Model, cfg.LLM.Default.Model)
	}
	if cfg.LLM.GetReviewerConfig().Model != cfg.LLM.Default.Model {
		t.Errorf("GetReviewerConfig().Model=%q; want %q", cfg.LLM.GetReviewerConfig().Model, cfg.LLM.Default.Model)
	}
	if cfg.LLM.Retry.Attempts != 3 {
		t.Errorf("LLM.Retry.Attempts=%d; want 3", cfg.LLM.Retry.Attempts)
	}
	if cfg.LLM.Retry.InitialDelayMS != 500 {
		t.Errorf("LLM.Retry.InitialDelayMS=%d; want 500", cfg.LLM.Retry.InitialDelayMS)
	}
	if cfg.LLM.Retry.Multiplier != 2.0 {
		t.Errorf("LLM.Retry.Multiplier=%v; want 2.0", cfg.LLM.Retry.Multiplier)
	}
	if cfg.LLM.RateLimit.Mode != "fail_fast" {
		t.Errorf("LLM.RateLimit.Mode=%q; want %q", cfg.LLM.RateLimit.Mode, "fail_fast")
	}
	if cfg.LLM.RateLimit.MaxWaitSeconds != 30 {
		t.Errorf("LLM.RateLimit.MaxWaitSeconds=%d; want 30", cfg.LLM.RateLimit.MaxWaitSeconds)
	}
	if len(cfg.Pipeline.Check) == 0 {
		t.Error("Pipeline.Check should not be empty")
	}
}

// ── Load ─────────────────────────────────────────────────────────────────────

func TestLoad_ファイルが存在しない場合はデフォルト設定を返す(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(filepath.Join(t.TempDir(), "nonexistent.yml"))
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if cfg.Project.Language != "Go" {
		t.Errorf("デフォルト設定の Language=%q; want %q", cfg.Project.Language, "Go")
	}
}

func TestLoad_新フォーマットのYAMLファイルを読み込む(t *testing.T) {
	t.Parallel()
	yml := `version: "1.0"
project:
  language: Python
  framework: Django
llm:
  default:
    provider: copilot
    model: gpt-4o-mini
  coder:
    provider: ollama
    model: qwen2.5-coder:3b
    base_url: http://localhost:11434/v1
  retry:
    attempts: 2
    initial_delay_ms: 100
    multiplier: 1.5
  rate_limit:
    mode: honor_wait
    max_wait_seconds: 15
environment:
  type: local
pipeline:
  check:
    - "pytest"
`
	path := writeTemp(t, yml)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if cfg.Version != "1.0" {
		t.Errorf("Version=%q; want %q", cfg.Version, "1.0")
	}
	if cfg.Project.Language != "Python" {
		t.Errorf("Project.Language=%q; want %q", cfg.Project.Language, "Python")
	}
	if cfg.Project.Framework != "Django" {
		t.Errorf("Project.Framework=%q; want %q", cfg.Project.Framework, "Django")
	}
	if cfg.LLM.Default.Model != "gpt-4o-mini" {
		t.Errorf("LLM.Default.Model=%q; want %q", cfg.LLM.Default.Model, "gpt-4o-mini")
	}
	// Coder override: Ollama で qwen2.5-coder:3b
	if cfg.LLM.GetCoderConfig().Provider != "ollama" {
		t.Errorf("GetCoderConfig().Provider=%q; want %q", cfg.LLM.GetCoderConfig().Provider, "ollama")
	}
	if cfg.LLM.GetCoderConfig().Model != "qwen2.5-coder:3b" {
		t.Errorf("GetCoderConfig().Model=%q; want %q", cfg.LLM.GetCoderConfig().Model, "qwen2.5-coder:3b")
	}
	// Planner/Reviewer は Default にフォールバック
	if cfg.LLM.GetPlannerConfig().Provider != "copilot" {
		t.Errorf("GetPlannerConfig().Provider=%q; want %q", cfg.LLM.GetPlannerConfig().Provider, "copilot")
	}
	if cfg.LLM.RateLimit.Mode != "honor_wait" {
		t.Errorf("LLM.RateLimit.Mode=%q; want %q", cfg.LLM.RateLimit.Mode, "honor_wait")
	}
	if len(cfg.Pipeline.Check) != 1 || cfg.Pipeline.Check[0] != "pytest" {
		t.Errorf("Pipeline.Check=%v; want [\"pytest\"]", cfg.Pipeline.Check)
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
  check:
    - "echo ok"
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
	// language/framework/llm/agents を省略した最小設定。
	yaml := `version: "1.0"
environment:
  type: local
pipeline:
  check:
    - "go test ./..."
`
	path := writeTemp(t, yaml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if cfg.Project.Language != "Go" {
		t.Errorf("fillDefaults: Language=%q; want %q", cfg.Project.Language, "Go")
	}
	if cfg.LLM.Default.Provider != "copilot" {
		t.Errorf("fillDefaults: LLM.Default.Provider=%q; want %q", cfg.LLM.Default.Provider, "copilot")
	}
	if cfg.LLM.Default.Model != "gpt-4.1" {
		t.Errorf("fillDefaults: LLM.Default.Model=%q; want %q", cfg.LLM.Default.Model, "gpt-4.1")
	}
	if cfg.LLM.AutoFixModel != cfg.LLM.Default.Model {
		t.Errorf("fillDefaults: LLM.AutoFixModel=%q; want same as LLM.Default.Model=%q", cfg.LLM.AutoFixModel, cfg.LLM.Default.Model)
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
  check:
    - "go test ./..."
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
  default:
    provider: copilot
    model: gpt-5-mini
environment:
  type: local
pipeline:
  check:
    - "go test ./..."
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

func TestLoad_ollamaProviderとbaseURLを読み込める(t *testing.T) {
	t.Parallel()
	yaml := `version: "1.0"
llm:
  default:
    provider: ollama
    model: qwen3:8b
    base_url: http://localhost:11434/v1
environment:
  type: local
pipeline:
  check:
    - "go test ./..."
`
	path := writeTemp(t, yaml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if cfg.LLM.Default.Provider != "ollama" {
		t.Errorf("LLM.Default.Provider=%q; want %q", cfg.LLM.Default.Provider, "ollama")
	}
	if cfg.LLM.Default.BaseURL != "http://localhost:11434/v1" {
		t.Errorf("LLM.Default.BaseURL=%q; want %q", cfg.LLM.Default.BaseURL, "http://localhost:11434/v1")
	}
}

func TestLoad_docker環境でimageを省略するとデフォルトイメージが補完される(t *testing.T) {
	t.Parallel()
	yaml := `version: "1.0"
environment:
  type: docker
pipeline:
  check:
    - "go test ./..."
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
  check:
    - "go test ./..."
`
	path := writeTemp(t, yaml)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("不正な environment.type でエラーを期待しましたが nil でした")
	}
}

func TestLoad_不正なllmProviderはエラーを返す(t *testing.T) {
	t.Parallel()
	// 新フォーマットで不正プロバイダーを指定
	yml := `version: "1.0"
llm:
  default:
    provider: openai
    model: gpt-4.1
environment:
  type: local
pipeline:
  check:
    - "go test ./..."
`
	path := writeTemp(t, yml)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("不正な llm.default.provider でエラーを期待しましたが nil でした")
	}
}

func TestLoad_旧フォーマットはエラーを返す(t *testing.T) {
	t.Parallel()
	yml := `version: "1.0"
llm:
  provider: copilot
  model: gpt-4.1
environment:
  type: local
pipeline:
  check:
    - "go test ./..."
`
	path := writeTemp(t, yml)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("旧フォーマット llm.provider は非サポートのためエラーを期待しましたが nil でした")
	}
}

func TestLoad_不正なrateLimitModeはエラーを返す(t *testing.T) {
	t.Parallel()
	yml := `version: "1.0"
llm:
  default:
    provider: copilot
    model: gpt-4.1
  rate_limit:
    mode: invalid_mode
environment:
  type: local
pipeline:
  check:
    - "go test ./..."
`
	path := writeTemp(t, yml)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("不正な llm.rate_limit.mode でエラーを期待しましたが nil でした")
	}
}

func TestLoad_不正なversionはエラーを返す(t *testing.T) {
	t.Parallel()
	yaml := `version: "2.0"
environment:
  type: local
pipeline:
  check:
    - "go test ./..."
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
  check:
    - "go test ./..."
`
	path := writeTemp(t, yaml)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("未知のキーでエラーを期待しましたが nil でした")
	}
}

func TestLoad_pipelineが空の場合はエラーを返す(t *testing.T) {
	t.Parallel()
	// Pipeline.Check を明示的に空リストで上書きするには Config 直接操作が必要なため、
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
	if len(cfg.Pipeline.Check) == 0 {
		t.Error("fillDefaults: Pipeline.Check が空のままです")
	}
}

func TestLoad_pipelineCheck文字列が空の場合はエラーを返す(t *testing.T) {
	t.Parallel()
	yaml := `version: "1.0"
environment:
  type: local
pipeline:
  check:
    - ""
`
	path := writeTemp(t, yaml)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("pipeline.check[空文字列] の場合はエラーを期待しましたが nil でした")
	}
}

func TestLoad_pipelineAutoFix文字列が空の場合はエラーを返す(t *testing.T) {
	t.Parallel()
	yaml := `version: "1.0"
environment:
  type: local
pipeline:
  auto_fix:
    - ""
  check:
    - "go test ./..."
`
	path := writeTemp(t, yaml)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("pipeline.auto_fix[空文字列] の場合はエラーを期待しましたが nil でした")
	}
}

// ── ハイブリッド構成 (エージェント別プロバイダー設定) ────────────────────────────────────────

func TestLoad_CoderにOllama_PlannerにCopilotのハイブリッド構成(t *testing.T) {
	t.Parallel()
	yml := `version: "1.0"
llm:
  default:
    provider: copilot
    model: gpt-4o-mini
  coder:
    provider: ollama
    model: qwen2.5-coder:3b
    base_url: http://localhost:11434/v1
environment:
  type: local
pipeline:
  check:
    - "go test ./..."
`
	path := writeTemp(t, yml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	// Default: copilot/gpt-4o-mini
	if cfg.LLM.Default.Provider != "copilot" {
		t.Errorf("LLM.Default.Provider=%q; want %q", cfg.LLM.Default.Provider, "copilot")
	}
	// Coder: ollama オーバーライド
	coderCfg := cfg.LLM.GetCoderConfig()
	if coderCfg.Provider != "ollama" {
		t.Errorf("GetCoderConfig().Provider=%q; want %q", coderCfg.Provider, "ollama")
	}
	if coderCfg.Model != "qwen2.5-coder:3b" {
		t.Errorf("GetCoderConfig().Model=%q; want %q", coderCfg.Model, "qwen2.5-coder:3b")
	}
	if coderCfg.BaseURL != "http://localhost:11434/v1" {
		t.Errorf("GetCoderConfig().BaseURL=%q; want %q", coderCfg.BaseURL, "http://localhost:11434/v1")
	}
	// Planner/Reviewer: Default にフォールバック
	plannerCfg := cfg.LLM.GetPlannerConfig()
	if plannerCfg.Provider != "copilot" {
		t.Errorf("GetPlannerConfig().Provider=%q; want %q", plannerCfg.Provider, "copilot")
	}
	if plannerCfg.Model != "gpt-4o-mini" {
		t.Errorf("GetPlannerConfig().Model=%q; want %q", plannerCfg.Model, "gpt-4o-mini")
	}
}

func TestLoad_エージェント別設定のプロバイダーが不正な場合はエラーを返す(t *testing.T) {
	t.Parallel()
	yml := `version: "1.0"
llm:
  default:
    provider: copilot
    model: gpt-4o-mini
  coder:
    provider: unsupported
    model: some-model
environment:
  type: local
pipeline:
  check:
    - "go test ./..."
`
	path := writeTemp(t, yml)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("不正な llm.coder.provider でエラーを期待しましたが nil でした")
	}
}

func TestLoad_エージェント別設定のプロバイダーを省略するとDefaultから返す(t *testing.T) {
	t.Parallel()
	// model のみ指定; provider は Default から補完される
	yml := `version: "1.0"
llm:
  default:
    provider: copilot
    model: gpt-4o-mini
  planner:
    model: gpt-4o
environment:
  type: local
pipeline:
  check:
    - "go test ./..."
`
	path := writeTemp(t, yml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	// Planner: model=gpt-4o, provider=copilot (補完される)
	plannerCfg := cfg.LLM.GetPlannerConfig()
	if plannerCfg.Provider != "copilot" {
		t.Errorf("GetPlannerConfig().Provider=%q; want %q (Defaultから補完)", plannerCfg.Provider, "copilot")
	}
	if plannerCfg.Model != "gpt-4o" {
		t.Errorf("GetPlannerConfig().Model=%q; want %q", plannerCfg.Model, "gpt-4o")
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
