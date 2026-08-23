package config

import (
	"testing"
	"time"
)

func TestAIKeyFallbackIsProviderScoped(t *testing.T) {
	t.Setenv("IOT_AI_PROVIDER", "openai-compatible")
	t.Setenv("IOT_AI_API_KEY", "")
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-secret")
	if got := Load().AIAPIKey; got != "" {
		t.Fatalf("DeepSeek key leaked into generic provider config: %q", got)
	}

	t.Setenv("IOT_AI_PROVIDER", "deepseek")
	if got := Load().AIAPIKey; got != "deepseek-secret" {
		t.Fatalf("DeepSeek provider key=%q", got)
	}

	t.Setenv("IOT_AI_API_KEY", "generic-secret")
	if got := Load().AIAPIKey; got != "generic-secret" {
		t.Fatalf("generic provider key did not take precedence: %q", got)
	}
}

func TestHarnessConfiguration(t *testing.T) {
	t.Setenv("IOT_AI_HARNESS_URL", "https://harness.example/")
	t.Setenv("IOT_AI_HARNESS_TOKEN", "service-token")
	t.Setenv("IOT_AI_HARNESS_MCP_URL", "https://api.example/mcp/harness")
	t.Setenv("IOT_AI_HARNESS_MODEL", "deepseek-chat")
	t.Setenv("IOT_AI_HARNESS_TIMEOUT", "45s")
	cfg := Load()
	if cfg.AIHarnessURL != "https://harness.example" || cfg.AIHarnessToken != "service-token" || cfg.AIHarnessMCPURL != "https://api.example/mcp/harness" || cfg.AIHarnessModel != "deepseek-chat" || cfg.AIHarnessTimeout != 45*time.Second {
		t.Fatalf("unexpected harness configuration: %#v", cfg)
	}
}
