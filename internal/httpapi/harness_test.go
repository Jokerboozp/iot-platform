package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"iot-platform/internal/adapters/local"
	"iot-platform/internal/adapters/memory"
	"iot-platform/internal/auth"
	"iot-platform/internal/config"
	"iot-platform/internal/core"
	"iot-platform/internal/metrics"
	"iot-platform/internal/parser"
	"iot-platform/internal/ports"
)

type captureWorkflowRuntime struct {
	mu       sync.Mutex
	requests []ports.AIWorkflowRequest
	fail     bool
}

func (f *captureWorkflowRuntime) ListWorkflows(context.Context) ([]ports.AIWorkflowPlugin, error) {
	return []ports.AIWorkflowPlugin{{ID: "ops-assistant", Name: "Ops", Enabled: true}}, nil
}

func (f *captureWorkflowRuntime) StreamChat(_ context.Context, in ports.AIWorkflowRequest, emit func(ports.AIWorkflowEvent) error) (ports.AIWorkflowResult, error) {
	f.mu.Lock()
	f.requests = append(f.requests, in)
	f.mu.Unlock()
	if f.fail {
		if emit != nil {
			_ = emit(ports.AIWorkflowEvent{Type: "run.started", RunID: in.RunID})
			_ = emit(ports.AIWorkflowEvent{Type: "run.failed", RunID: in.RunID, Code: "HARNESS_FAILED", Message: "Harness runtime request failed"})
		}
		return ports.AIWorkflowResult{RunID: in.RunID}, errors.New("sensitive internal runtime error")
	}
	for _, event := range []ports.AIWorkflowEvent{
		{Type: "run.started", RunID: in.RunID, WorkflowID: in.WorkflowID, Data: map[string]any{
			"conversationId": in.ConversationID,
			"visible":        "ok",
			"nested": map[string]any{
				"sessionId": "internal-session",
				"items":     []any{map[string]any{"api_key": "internal-api-key", "safe": "nested-ok"}},
			},
		}},
		{Type: "text.delta", RunID: in.RunID, Delta: "safe "},
		{Type: "run.completed", RunID: in.RunID, WorkflowID: in.WorkflowID, Answer: "safe answer"},
	} {
		if emit != nil {
			if err := emit(event); err != nil {
				return ports.AIWorkflowResult{RunID: in.RunID}, err
			}
		}
	}
	return ports.AIWorkflowResult{RunID: in.RunID, WorkflowID: in.WorkflowID, Answer: "safe answer"}, nil
}

func (*captureWorkflowRuntime) Health(context.Context) error { return nil }

func TestHarnessHTTPBridgeAndTenantScopedConversation(t *testing.T) {
	repo := memory.NewRepository()
	archive, err := local.NewArchive(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	engine := core.New(repo, archive, local.NewBus(), local.NewRealtime(), parser.NewRegistry(parser.JSONParser{}), slog.New(slog.NewTextHandler(io.Discard, nil)))
	runtime := &captureWorkflowRuntime{}
	engine.AIWorkflows = runtime
	registry := metrics.New()
	cfg := config.Load()
	cfg.JWTSecret = "test-secret-at-least-32-characters"
	api := New(cfg, engine, registry, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	token, err := api.auth.Issue("alice", "tenant-a", "viewer", nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	workflows := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/ai/workflows", token, nil, http.StatusOK)
	if workflows["configured"] != true || workflows["healthy"] != true || workflows["count"].(float64) != 1 {
		t.Fatalf("unexpected workflows response: %#v", workflows)
	}
	chat := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/ai/chat", token, map[string]any{"question": "status?", "workflowId": "ops-assistant", "conversationId": "browser-controlled", "model": "deepseek-chat", "maxTokens": 99999}, http.StatusOK)
	if chat["answer"] != "safe answer" || !strings.HasPrefix(chat["runId"].(string), "ai_run_") {
		t.Fatalf("unexpected rich chat response: %#v", chat)
	}
	if _, leaked := chat["mcpToken"]; leaked {
		t.Fatalf("MCP credential leaked to browser: %#v", chat)
	}

	runtime.mu.Lock()
	captured := runtime.requests[0]
	runtime.mu.Unlock()
	claims, err := api.auth.Parse(captured.MCPToken)
	if err != nil {
		t.Fatal(err)
	}
	if claims.TokenUse != "harness" || claims.RunID != captured.RunID || !claims.HasAudience(auth.HarnessAudience) || len(claims.Scopes) != len(auth.HarnessReadScopes()) {
		t.Fatalf("unsafe harness token: %#v", claims)
	}
	if captured.ConversationID == "browser-controlled" || captured.ConversationID != harnessConversationID("tenant-a", "alice", "browser-controlled") {
		t.Fatalf("conversation ID was not tenant scoped: %q", captured.ConversationID)
	}
	if captured.MaxTokens != 8192 {
		t.Fatalf("maxTokens was not clamped: %d", captured.MaxTokens)
	}
	if captured.Model != "deepseek-chat" {
		t.Fatalf("model was not passed to harness: %q", captured.Model)
	}

	streamReq, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/ai/chat/stream", bytes.NewBufferString(`{"question":"stream?","conversationId":"browser-controlled"}`))
	streamReq.Header.Set("Authorization", "Bearer "+token)
	streamReq.Header.Set("Content-Type", "application/json")
	streamResp, err := server.Client().Do(streamReq)
	if err != nil {
		t.Fatal(err)
	}
	streamBody, _ := io.ReadAll(streamResp.Body)
	streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK || !strings.Contains(string(streamBody), "event: run.started") || !strings.Contains(string(streamBody), "event: text.delta") || !strings.Contains(string(streamBody), "event: run.completed") {
		t.Fatalf("unexpected SSE response: status=%d body=%s", streamResp.StatusCode, streamBody)
	}
	if strings.Contains(string(streamBody), "conv_") || strings.Contains(string(streamBody), "internal-session") || strings.Contains(string(streamBody), "internal-api-key") || !strings.Contains(string(streamBody), `"visible":"ok"`) || !strings.Contains(string(streamBody), `"safe":"nested-ok"`) {
		t.Fatalf("SSE leaked internal conversation/session data: %s", streamBody)
	}

	runtime.fail = true
	failedSync := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/ai/chat", token, map[string]any{"question": "fail?"}, http.StatusBadGateway)
	if errorMessage, _ := failedSync["detail"].(string); errorMessage != "AI workflow request failed" || strings.Contains(errorMessage, "sensitive internal runtime error") {
		t.Fatalf("sync workflow leaked an internal error: %#v", failedSync)
	}
	failedReq, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/ai/chat/stream", bytes.NewBufferString(`{"question":"fail?"}`))
	failedReq.Header.Set("Authorization", "Bearer "+token)
	failedReq.Header.Set("Content-Type", "application/json")
	failedResp, err := server.Client().Do(failedReq)
	if err != nil {
		t.Fatal(err)
	}
	failedBody, _ := io.ReadAll(failedResp.Body)
	failedResp.Body.Close()
	if count := strings.Count(string(failedBody), "event: run.failed"); count != 1 || strings.Contains(string(failedBody), "sensitive internal runtime error") {
		t.Fatalf("unsafe or duplicate terminal event: %s", failedBody)
	}

	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/mcp/harness", token, map[string]any{}, http.StatusForbidden)
	getMCP, _ := http.NewRequest(http.MethodGet, server.URL+"/mcp/harness", nil)
	getMCP.Header.Set("Authorization", "Bearer "+token)
	getMCPResp, err := server.Client().Do(getMCP)
	if err != nil {
		t.Fatal(err)
	}
	getMCPResp.Body.Close()
	if getMCPResp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET /mcp/harness status=%d", getMCPResp.StatusCode)
	}
	harnessToken, err := api.auth.IssueHarness("alice", "tenant-a", "run-x", auth.HarnessReadScopes(), 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/ai/workflows", harnessToken, nil, http.StatusForbidden)
}

func TestHarnessConversationIDIsStableAndTenantScoped(t *testing.T) {
	a := harnessConversationID("tenant-a", "alice", "conversation-1")
	if a != harnessConversationID("tenant-a", "alice", "conversation-1") {
		t.Fatal("conversation derivation is not stable")
	}
	if a == harnessConversationID("tenant-b", "alice", "conversation-1") || a == harnessConversationID("tenant-a", "bob", "conversation-1") {
		t.Fatal("conversation derivation is not tenant/user scoped")
	}
	if !strings.HasPrefix(a, "conv_") || strings.Contains(a, "tenant-a") || strings.Contains(a, "alice") {
		t.Fatalf("conversation derivation leaks identity: %q", a)
	}
}
