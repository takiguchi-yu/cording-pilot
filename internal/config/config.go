// Package config は .cording-pilot.yml を読み込み、
// パイプライン設定を提供します。
package config

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// DefaultConfigFileName はプロジェクトルートに配置する設定ファイルのデフォルト名です。
const DefaultConfigFileName = ".cording-pilot.yml"

const (
	defaultConfigVersion = "1.0"
	defaultLLMProvider   = "copilot"
	defaultLLMModel      = "gpt-4.1"
)

// Project はターゲット言語とテストフレームワークの設定を保持します。
type Project struct {
	// Language は対象プログラミング言語です（例: "go", "python", "typescript"）。
	Language string `yaml:"language"`
	// TestFramework はテストフレームワーク名です（例: "standard testing", "pytest", "jest"）。
	TestFramework string `yaml:"test_framework"`
}

// Agents は各 AI エージェントのシステムプロンプトを保持します。
type Agents struct {
	// Planner は計画エージェントのシステムプロンプトです。
	Planner string `yaml:"planner"`
	// Coder は実装エージェントのシステムプロンプトです。
	Coder string `yaml:"coder"`
	// Reviewer はレビューエージェントのシステムプロンプトです。
	Reviewer string `yaml:"reviewer"`
}

// LLM は LLM プロバイダーとモデルの設定を保持します。
type LLM struct {
	// Provider は使用する LLM プロバイダーです（例: "copilot", "openai"）。
	Provider string `yaml:"provider"`
	// Model は使用するモデル名です（例: "gpt-4o", "claude-3-5-sonnet-20240620"）。
	Model string `yaml:"model"`
	// AutoFixModel は自己修復フェーズで使用する軽量モデル名です（例: "gpt-4o-mini"）。
	// 省略時は Model と同じ値が使用されます。
	AutoFixModel string `yaml:"auto_fix_model,omitempty"`
	// PlannerClarificationModel は Interactive の質問生成で使用するモデル名です。
	PlannerClarificationModel string `yaml:"planner_clarification_model,omitempty"`
	// PlannerPlanModel は Plan/CompileIssue で使用するモデル名です。
	PlannerPlanModel string `yaml:"planner_plan_model,omitempty"`
	// CoderModel は実装フェーズ（構造化出力）で使用するモデル名です。
	CoderModel string `yaml:"coder_model,omitempty"`
	// ReviewerModel はレビューフェーズで使用するモデル名です。
	ReviewerModel string `yaml:"reviewer_model,omitempty"`
	// Retry は LLM 呼び出し時のリトライ設定です。
	Retry LLMRetry `yaml:"retry,omitempty"`
	// RateLimit は 429 応答時の制御設定です。
	RateLimit LLMRateLimit `yaml:"rate_limit,omitempty"`
}

// LLMRetry は LLM 呼び出し時の再試行ポリシーです。
type LLMRetry struct {
	Attempts       int     `yaml:"attempts,omitempty"`
	InitialDelayMS int     `yaml:"initial_delay_ms,omitempty"`
	Multiplier     float64 `yaml:"multiplier,omitempty"`
}

// LLMRateLimit は 429 応答時の制御設定です。
type LLMRateLimit struct {
	// Mode は fail_fast または honor_wait のいずれかです。
	Mode string `yaml:"mode,omitempty"`
	// MaxWaitSeconds は honor_wait 時に許容する待機秒数の上限です。
	MaxWaitSeconds int `yaml:"max_wait_seconds,omitempty"`
}

// Environment は実行環境に関する設定を保持します。
type Environment struct {
	// Type は使用する実行環境の種類です（"local", "docker", "nix"）。
	// 省略時は "local" が使用されます。
	Type string `yaml:"type"`
	// Image は使用する Docker イメージ名です（例: "golang:1.22"）。
	// type が "docker" の場合のみ参照されます。
	Image string `yaml:"image"`
}

// PipelineStep は品質チェックパイプラインの 1 ステップを表します。
type PipelineStep struct {
	// Name はステップの識別名です（ログ出力に使用されます）。
	Name string `yaml:"name"`
	// Command はコンテナ内で実行するシェルコマンド文字列です。
	Command string `yaml:"command"`
}

// Config は .cording-pilot.yml のトップレベル構造体です。
type Config struct {
	// Version は設定ファイルのスキーマバージョンです。
	Version string `yaml:"version"`
	// Project はターゲット言語とテストフレームワークの設定です。
	Project Project `yaml:"project"`
	// Agents は各 AI エージェントのシステムプロンプト設定です。
	Agents Agents `yaml:"agents"`
	// LLM は LLM プロバイダーとモデルの設定です。
	LLM LLM `yaml:"llm"`
	// Environment は実行環境の設定です。
	Environment Environment `yaml:"environment"`
	// AutoFix は品質チェック実行直前の自動修復コマンドのリストです。
	// 形式は Pipeline と同等です。失敗しても処理は続行します。
	AutoFix []PipelineStep `yaml:"auto_fix"`
	// Pipeline は順序が保証されたコマンドのリストです。
	Pipeline []PipelineStep `yaml:"pipeline"`
}

// DefaultGoConfig は .cording-pilot.yml が存在しない場合に使用する Go 向けデフォルト設定です。
func DefaultGoConfig() *Config {
	return &Config{
		Version: defaultConfigVersion,
		Project: Project{
			Language:      "go",
			TestFramework: "standard testing",
		},
		Agents: Agents{
			Planner:  defaultPlannerPrompt,
			Coder:    defaultCoderPrompt,
			Reviewer: defaultReviewerPrompt,
		},
		LLM: LLM{
			Provider:                  defaultLLMProvider,
			Model:                     defaultLLMModel,
			AutoFixModel:              "gpt-5-mini",
			PlannerClarificationModel: defaultLLMModel,
			PlannerPlanModel:          defaultLLMModel,
			CoderModel:                defaultLLMModel,
			ReviewerModel:             defaultLLMModel,
			Retry: LLMRetry{
				Attempts:       3,
				InitialDelayMS: 500,
				Multiplier:     2.0,
			},
			RateLimit: LLMRateLimit{
				Mode:           "fail_fast",
				MaxWaitSeconds: 30,
			},
		},
		Environment: Environment{
			Image: "golangci/golangci-lint:latest",
		},
		AutoFix: []PipelineStep{
			{Name: "tidy", Command: "go mod tidy"},
			{Name: "format", Command: "go fmt ./..."},
		},
		Pipeline: []PipelineStep{
			{Name: "goimports", Command: "goimports -w ."},
			{Name: "format", Command: "go fmt ./..."},
			{Name: "typecheck", Command: "go build ./..."},
			{Name: "lint", Command: "golangci-lint run"},
			{Name: "test", Command: "go test -v ./..."},
		},
	}
}

const defaultPlannerPrompt = `あなたは優秀なソフトウェアアーキテクトです。
与えられた要件を分析し、実装計画（目的・仕様・影響範囲）を日本語のMarkdown形式で出力してください。`

const defaultCoderPrompt = `あなたは熟練のエンジニアです。
テストコードまたはプロダクトコードの生成を求められます。
余分な説明やMarkdownコードブロックは含めず、以下のJSONスキーマに厳密に従って出力してください。

{"files":[{"path":"ファイルのパス","content":"ファイルの内容"}]}

- path にはリポジトリルートからの相対パスを指定してください。
- content にはファイルの完全な内容を文字列として指定してください。
- ファイルの拡張子やディレクトリ構造は、対象言語のベストプラクティスおよび既存のリポジトリ構成に従うこと。`

const defaultReviewerPrompt = `あなたは厳格なコードレビュアーです。
差分と要件を突き合わせてレビューし、結果を "Approve" または "Request Changes" のいずれかで冒頭に明示してください。
問題点がある場合は具体的な修正点を列挙してください。`

// Load は path で指定した YAML ファイルを読み込み、Config を返します。
// ファイルが存在しない場合は DefaultGoConfig を返します。
// ファイルが存在するが内容が不正な場合はエラーを返します。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 — path はユーザー指定の設定ファイル
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultGoConfig(), nil
		}
		return nil, fmt.Errorf("config: read file %q: %w", path, err)
	}

	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err = dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config: parse yaml %q: %w", path, err)
	}
	var trailing any
	if err = dec.Decode(&trailing); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("config: parse yaml %q: %w", path, err)
		}
		return nil, fmt.Errorf("config: parse yaml %q: multiple YAML documents are not supported", path)
	}

	cfg.fillDefaults()

	if err = cfg.validate(); err != nil {
		return nil, fmt.Errorf("config: validate %q: %w", path, err)
	}
	return &cfg, nil
}

// fillDefaults は設定ファイルの省略されたフィールドにデフォルト値を設定します。
func (c *Config) fillDefaults() {
	if c.Version == "" {
		c.Version = defaultConfigVersion
	}
	if c.Project.Language == "" {
		c.Project.Language = "go"
	}
	if c.Project.TestFramework == "" {
		c.Project.TestFramework = "standard testing"
	}
	if c.Agents.Planner == "" {
		c.Agents.Planner = defaultPlannerPrompt
	}
	if c.Agents.Coder == "" {
		c.Agents.Coder = defaultCoderPrompt
	}
	if c.Agents.Reviewer == "" {
		c.Agents.Reviewer = defaultReviewerPrompt
	}
	if c.LLM.Provider == "" {
		c.LLM.Provider = defaultLLMProvider
	}
	if c.LLM.Model == "" {
		c.LLM.Model = defaultLLMModel
	}
	if c.LLM.AutoFixModel == "" {
		c.LLM.AutoFixModel = c.LLM.Model
	}
	if c.LLM.PlannerClarificationModel == "" {
		c.LLM.PlannerClarificationModel = c.LLM.Model
	}
	if c.LLM.PlannerPlanModel == "" {
		c.LLM.PlannerPlanModel = c.LLM.Model
	}
	if c.LLM.CoderModel == "" {
		c.LLM.CoderModel = c.LLM.Model
	}
	if c.LLM.ReviewerModel == "" {
		c.LLM.ReviewerModel = c.LLM.Model
	}
	if c.LLM.Retry.Attempts == 0 {
		c.LLM.Retry.Attempts = 3
	}
	if c.LLM.Retry.InitialDelayMS == 0 {
		c.LLM.Retry.InitialDelayMS = 500
	}
	if c.LLM.Retry.Multiplier == 0 {
		c.LLM.Retry.Multiplier = 2.0
	}
	if c.LLM.RateLimit.Mode == "" {
		c.LLM.RateLimit.Mode = "fail_fast"
	}
	if c.LLM.RateLimit.MaxWaitSeconds == 0 {
		c.LLM.RateLimit.MaxWaitSeconds = 30
	}
	if c.Environment.Type == "" {
		c.Environment.Type = "local"
	}
	if c.Environment.Type == "docker" && c.Environment.Image == "" {
		c.Environment.Image = DefaultGoConfig().Environment.Image
	}
	if len(c.AutoFix) == 0 {
		c.AutoFix = DefaultGoConfig().AutoFix
	}
	if len(c.Pipeline) == 0 {
		c.Pipeline = DefaultGoConfig().Pipeline
	}
}

// validate は Config の内容を検証します。
func (c *Config) validate() error {
	if c.Version != defaultConfigVersion {
		return fmt.Errorf("version must be %q; got %q", defaultConfigVersion, c.Version)
	}
	if c.LLM.Provider != defaultLLMProvider {
		return fmt.Errorf("llm.provider must be %q; got %q", defaultLLMProvider, c.LLM.Provider)
	}
	if c.LLM.Retry.Attempts < 1 {
		return fmt.Errorf("llm.retry.attempts must be >= 1; got %d", c.LLM.Retry.Attempts)
	}
	if c.LLM.Retry.InitialDelayMS < 0 {
		return fmt.Errorf("llm.retry.initial_delay_ms must be >= 0; got %d", c.LLM.Retry.InitialDelayMS)
	}
	if c.LLM.Retry.Multiplier < 1.0 {
		return fmt.Errorf("llm.retry.multiplier must be >= 1.0; got %v", c.LLM.Retry.Multiplier)
	}
	switch c.LLM.RateLimit.Mode {
	case "fail_fast", "honor_wait":
	default:
		return fmt.Errorf("llm.rate_limit.mode must be one of \"fail_fast\", \"honor_wait\"; got %q", c.LLM.RateLimit.Mode)
	}
	if c.LLM.RateLimit.MaxWaitSeconds < 0 {
		return fmt.Errorf("llm.rate_limit.max_wait_seconds must be >= 0; got %d", c.LLM.RateLimit.MaxWaitSeconds)
	}

	switch c.Environment.Type {
	case "local", "nix":
		// Image フィールドは不要。
	case "docker":
		if c.Environment.Image == "" {
			return fmt.Errorf("environment.image must not be empty when environment.type is \"docker\"")
		}
	default:
		return fmt.Errorf("environment.type must be one of \"local\", \"docker\", \"nix\"; got %q", c.Environment.Type)
	}
	if len(c.Pipeline) == 0 {
		return fmt.Errorf("pipeline must contain at least one step")
	}
	for i, step := range c.AutoFix {
		if step.Name == "" {
			return fmt.Errorf("auto_fix[%d].name must not be empty", i)
		}
		if step.Command == "" {
			return fmt.Errorf("auto_fix[%d].command must not be empty", i)
		}
	}
	for i, step := range c.Pipeline {
		if step.Name == "" {
			return fmt.Errorf("pipeline[%d].name must not be empty", i)
		}
		if step.Command == "" {
			return fmt.Errorf("pipeline[%d].command must not be empty", i)
		}
	}
	return nil
}
