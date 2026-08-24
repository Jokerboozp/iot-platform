package aiadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"iot-platform/internal/ports"
)

func TestHarnessClientSeparatesCredentialsAndParsesNDJSON(t *testing.T) {
	const serviceToken = "0123456789abcdef0123456789abcdef"
	var streamBody map[string]any
	var savedManifest ports.AIWorkflowManifest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-IOT-Harness-Token") != serviceToken {
			t.Errorf("missing service credential: %q", r.Header.Get("X-IOT-Harness-Token"))
		}
		switch r.URL.Path {
		case "/v1/plugins":
			if r.Header.Get("Authorization") != "" {
				t.Errorf("plugins request unexpectedly contains MCP credential")
			}
			if r.Method == http.MethodPost {
				if err := json.NewDecoder(r.Body).Decode(&savedManifest); err != nil {
					t.Fatal(err)
				}
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]any{"id": savedManifest.ID, "name": savedManifest.Name, "enabled": savedManifest.Enabled})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{{"id": "ops-assistant", "name": "Ops", "enabled": true, "defaultModel": "deepseek-v4-flash", "maxTokens": 16384}}})
		case "/v1/chat/stream":
			if r.Header.Get("Authorization") != "Bearer short-mcp-jwt" {
				t.Errorf("MCP JWT sent in wrong header: %q", r.Header.Get("Authorization"))
			}
			if err := json.NewDecoder(r.Body).Decode(&streamBody); err != nil {
				t.Fatal(err)
			}
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = fmt.Fprintln(w, `{"type":"run.started"}`)
			_, _ = fmt.Fprintln(w, `{"type":"text.delta","delta":"hello "}`)
			_, _ = fmt.Fprintln(w, `{"type":"tool.completed","callId":"call-1","tool":"query_alarm_list","success":true}`)
			_, _ = fmt.Fprintln(w, `{"type":"text.delta","delta":"world"}`)
			_, _ = fmt.Fprintln(w, `{"type":"run.completed","conversationId":"internal-conversation"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewHarness(server.URL, serviceToken, "https://api.example/mcp/harness", "deepseek-chat", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	plugins, err := client.ListWorkflows(context.Background())
	if err != nil || len(plugins) != 1 || plugins[0].ID != "ops-assistant" || plugins[0].DefaultModel != "deepseek-v4-flash" || plugins[0].MaxTokens != 16384 {
		t.Fatalf("plugins=%#v err=%v", plugins, err)
	}
	created, err := client.SaveWorkflow(context.Background(), ports.AIWorkflowManifest{SchemaVersion: 1, ID: "dynamic", Name: "Dynamic", Description: "Dynamic Agent", Version: "1.0.0", Enabled: true, Persona: "Read-only status agent", DefaultModel: "deepseek-chat", MaxTokens: 2048, Capabilities: []string{"status"}, AllowedTools: []string{"mcp__iot__query_system_overview"}})
	if err != nil || created.ID != "dynamic" || savedManifest.ID != "dynamic" {
		t.Fatalf("created=%#v saved=%#v err=%v", created, savedManifest, err)
	}
	var types []string
	result, err := client.StreamChat(context.Background(), ports.AIWorkflowRequest{RunID: "run-1", ConversationID: "conv-1", WorkflowID: "ops-assistant", Question: "status?", MCPToken: "short-mcp-jwt"}, func(event ports.AIWorkflowEvent) error {
		types = append(types, event.Type)
		return nil
	})
	if err != nil || result.Answer != "hello world" || strings.Join(types, ",") != "run.started,text.delta,tool.completed,text.delta,run.completed" {
		t.Fatalf("result=%#v events=%v err=%v", result, types, err)
	}
	if _, leaked := streamBody["mcpToken"]; leaked {
		t.Fatalf("short-lived MCP token leaked into JSON body: %#v", streamBody)
	}
	for _, field := range []string{"runId", "conversationId", "workflowId", "question", "mcpUrl", "model", "maxTokens"} {
		if _, ok := streamBody[field]; !ok {
			t.Fatalf("missing sidecar field %q in %#v", field, streamBody)
		}
	}
}

func TestHarnessClientRejectsUnknownEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, `{"type":"debug.secret","text":"nope"}`)
	}))
	defer server.Close()
	client, err := NewHarness(server.URL, "0123456789abcdef0123456789abcdef", "https://api.example/mcp/harness", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.StreamChat(context.Background(), ports.AIWorkflowRequest{RunID: "run-1", Question: "q", MCPToken: "jwt"}, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported harness event type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHarnessClientRejectsWeakServiceToken(t *testing.T) {
	if _, err := NewHarness("https://harness.example", "too-short", "https://api.example/mcp/harness", "", time.Second); err == nil {
		t.Fatal("weak harness service token was accepted")
	}
}
