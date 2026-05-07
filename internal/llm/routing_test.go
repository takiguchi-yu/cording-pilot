package llm_test

import (
	"context"
	"testing"

	"github.com/takiguchi-yu/cording-pilot/internal/llm"
)

type routingStubClient struct {
	generateCalled           int
	generateStructuredCalled int
}

func (s *routingStubClient) Generate(_ context.Context, _ string) (string, error) {
	s.generateCalled++
	return "ok", nil
}

func (s *routingStubClient) GenerateStructured(_ context.Context, _ string, _ interface{}) error {
	s.generateStructuredCalled++
	return nil
}

func TestRoutingClient_Generateはタグで送信先を切り替える(t *testing.T) {
	t.Parallel()

	defaultClient := &routingStubClient{}
	plannerPlan := &routingStubClient{}
	reviewer := &routingStubClient{}

	c := llm.NewRoutingClient(defaultClient, llm.RoleClients{
		PlannerPlan: plannerPlan,
		Reviewer:    reviewer,
	})

	if _, err := c.Generate(context.Background(), "[PLAN] 計画"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := c.Generate(context.Background(), "[REVIEW] 査読"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := c.Generate(context.Background(), "通常タスク"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plannerPlan.generateCalled != 1 {
		t.Errorf("plannerPlan.generateCalled=%d; want 1", plannerPlan.generateCalled)
	}
	if reviewer.generateCalled != 1 {
		t.Errorf("reviewer.generateCalled=%d; want 1", reviewer.generateCalled)
	}
	if defaultClient.generateCalled != 1 {
		t.Errorf("defaultClient.generateCalled=%d; want 1", defaultClient.generateCalled)
	}
}

func TestRoutingClient_GenerateStructuredはタグで送信先を切り替える(t *testing.T) {
	t.Parallel()

	defaultClient := &routingStubClient{}
	clarification := &routingStubClient{}
	coder := &routingStubClient{}

	c := llm.NewRoutingClient(defaultClient, llm.RoleClients{
		PlannerClarification: clarification,
		Coder:                coder,
	})

	var target map[string]any
	if err := c.GenerateStructured(context.Background(), "[CLARIFY] 質問生成", &target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := c.GenerateStructured(context.Background(), "[TEST_GEN] テスト生成", &target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if clarification.generateStructuredCalled != 1 {
		t.Errorf("clarification.generateStructuredCalled=%d; want 1", clarification.generateStructuredCalled)
	}
	if coder.generateStructuredCalled != 1 {
		t.Errorf("coder.generateStructuredCalled=%d; want 1", coder.generateStructuredCalled)
	}
	if defaultClient.generateStructuredCalled != 0 {
		t.Errorf("defaultClient.generateStructuredCalled=%d; want 0", defaultClient.generateStructuredCalled)
	}
}
