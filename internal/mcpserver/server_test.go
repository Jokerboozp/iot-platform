package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"iot-platform/internal/auth"
	"iot-platform/internal/core"
)

func TestHarnessToolSurfaceIsReadOnlyAndScopeChecked(t *testing.T) {
	handler := NewHarness(&core.Engine{})
	claims := auth.Claims{
		Username: "alice",
		TenantID: "tenant-a",
		TokenUse: "harness",
		RunID:    "run-1",
		Scopes:   []string{auth.ScopeQueryDeviceLatest},
		RegisteredClaims: jwt.RegisteredClaims{
			Audience: jwt.ClaimStrings{auth.HarnessAudience},
		},
	}

	listBody := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	list := httptest.NewRequest(http.MethodPost, "http://localhost/mcp/harness", strings.NewReader(listBody))
	list.Header.Set("Content-Type", "application/json")
	list = list.WithContext(auth.ContextWithClaims(context.Background(), claims))
	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, list)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("tools/list status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
	body := listResponse.Body.String()
	for _, tool := range []string{"query_device_latest", "query_alarm_list", "query_property_history", "query_similar_alarms", "query_knowledge_base"} {
		if !strings.Contains(body, `"name":"`+tool+`"`) {
			t.Fatalf("missing read tool %q: %s", tool, body)
		}
	}
	if strings.Contains(body, "create_rule_draft") {
		t.Fatalf("write-capable tool leaked into harness surface: %s", body)
	}

	callBody := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"query_alarm_list","arguments":{}}}`
	call := httptest.NewRequest(http.MethodPost, "http://localhost/mcp/harness", strings.NewReader(callBody))
	call.Header.Set("Content-Type", "application/json")
	call = call.WithContext(auth.ContextWithClaims(context.Background(), claims))
	callResponse := httptest.NewRecorder()
	handler.ServeHTTP(callResponse, call)
	if callResponse.Code != http.StatusOK || !strings.Contains(callResponse.Body.String(), "not authorized") {
		t.Fatalf("out-of-scope tool call was not rejected: status=%d body=%s", callResponse.Code, callResponse.Body.String())
	}
}

func TestBoundedLimit(t *testing.T) {
	for _, test := range []struct{ value, fallback, maximum, want int }{
		{0, 20, 50, 20}, {-1, 20, 50, 20}, {10, 20, 50, 10}, {1000, 20, 50, 50},
	} {
		if got := boundedLimit(test.value, test.fallback, test.maximum); got != test.want {
			t.Fatalf("boundedLimit(%d, %d, %d)=%d want=%d", test.value, test.fallback, test.maximum, got, test.want)
		}
	}
}
