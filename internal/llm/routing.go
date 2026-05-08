package llm

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/takiguchi-yu/cording-pilot/internal/config"
	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

// NewClient は LLMProviderConfig から適切な LLM クライアントを生成します。
// provider に応じて CopilotClient または OllamaClient を返します。
// CopilotOptions は Copilot クライアントのリトライ・レートリミット設定に使用されます。
func NewClient(providerCfg config.LLMProviderConfig, log *logger.Logger, opts CopilotOptions) (Client, error) {
	switch providerCfg.Provider {
	case "copilot":
		token := os.Getenv("GITHUB_TOKEN")
		if token == "" {
			return nil, fmt.Errorf("provider %q には GITHUB_TOKEN 環境変数が必要です", providerCfg.Provider)
		}
		return NewCopilotClientWithOptions(providerCfg.Model, token, log, opts)
	case "ollama":
		baseURL := strings.TrimSpace(providerCfg.BaseURL)
		if baseURL == "" {
			baseURL = DefaultOllamaBaseURL
		}
		return NewOllamaClient(providerCfg.Model, baseURL, log)
	default:
		return nil, fmt.Errorf("未対応の LLM プロバイダーです: %q (対応プロバイダー: copilot, ollama)", providerCfg.Provider)
	}
}

// RoleClients はフェーズ/役割ごとに使用する LLM クライアントを保持します。
type RoleClients struct {
	PlannerClarification Client
	PlannerPlan          Client
	Coder                Client
	Reviewer             Client
}

// routingClient はプロンプトタグに応じて送信先の LLM クライアントを切り替えます。
type routingClient struct {
	defaultClient Client
	roles         RoleClients
}

// NewRoutingClient は役割別クライアントを選択する Client を返します。
func NewRoutingClient(defaultClient Client, roles RoleClients) Client {
	if defaultClient == nil {
		switch {
		case roles.Coder != nil:
			defaultClient = roles.Coder
		case roles.PlannerPlan != nil:
			defaultClient = roles.PlannerPlan
		case roles.PlannerClarification != nil:
			defaultClient = roles.PlannerClarification
		case roles.Reviewer != nil:
			defaultClient = roles.Reviewer
		}
	}
	return &routingClient{defaultClient: defaultClient, roles: roles}
}

// Generate implements Client.
func (c *routingClient) Generate(ctx context.Context, prompt string) (string, error) {
	return c.chooseGenerateClient(prompt).Generate(ctx, prompt)
}

// GenerateStructured implements Client.
func (c *routingClient) GenerateStructured(ctx context.Context, prompt string, target interface{}) error {
	return c.chooseStructuredClient(prompt).GenerateStructured(ctx, prompt, target)
}

func (c *routingClient) chooseGenerateClient(prompt string) Client {
	lower := strings.ToLower(prompt)
	switch {
	case strings.Contains(lower, "[review]"):
		if c.roles.Reviewer != nil {
			return c.roles.Reviewer
		}
	case strings.Contains(lower, "[plan]") || strings.Contains(lower, "[compile_issue]"):
		if c.roles.PlannerPlan != nil {
			return c.roles.PlannerPlan
		}
	}
	return c.defaultOrSelf()
}

func (c *routingClient) chooseStructuredClient(prompt string) Client {
	lower := strings.ToLower(prompt)
	switch {
	case strings.Contains(lower, "[clarify]"):
		if c.roles.PlannerClarification != nil {
			return c.roles.PlannerClarification
		}
	default:
		if c.roles.Coder != nil {
			return c.roles.Coder
		}
	}
	return c.defaultOrSelf()
}

func (c *routingClient) defaultOrSelf() Client {
	return c.defaultClient
}
