package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"iot-platform/internal/adapters/local"
	"iot-platform/internal/adapters/memory"
	"iot-platform/internal/config"
	"iot-platform/internal/core"
	"iot-platform/internal/metrics"
	"iot-platform/internal/model"
	"iot-platform/internal/parser"
)

type protocolEndpointAI struct{}

func (protocolEndpointAI) AnalyzeAlarm(context.Context, model.Alarm, []map[string]any, []string) (model.AIAnalysis, error) {
	return model.AIAnalysis{}, nil
}
func (protocolEndpointAI) Chat(context.Context, string, string) (string, error) { return "ok", nil }
func (protocolEndpointAI) RuleDraft(context.Context, string, string) (model.AlarmRule, error) {
	return model.AlarmRule{}, nil
}
func (protocolEndpointAI) Health(context.Context) error { return nil }
func (protocolEndpointAI) GenerateJSON(context.Context, string, string, string) (string, error) {
	return `{"name":"端点测试协议","protocol":"endpoint-hex","transport":"TCP","payloadFormat":"hex","messageType":"PROPERTY_REPORT","setup":"const bytes = hexToBytes(raw.payload)","fields":[{"name":"temperature","expression":"bytes[1] / 10"}]}`, nil
}

func TestProtocolAssistantEndpoints(t *testing.T) {
	repo := memory.NewRepository()
	archive, err := local.NewArchive(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	engine := core.New(repo, archive, local.NewBus(), local.NewRealtime(), parser.NewRegistry(parser.JavaScriptParser{}), slog.New(slog.NewTextHandler(io.Discard, nil)))
	engine.AI = protocolEndpointAI{}
	engine.Metrics = metrics.New()
	if err = engine.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	cfg := config.Load()
	cfg.DevMode = true
	cfg.JWTSecret = "test-secret-at-least-32-characters"
	api := New(cfg, engine, engine.Metrics.(*metrics.Registry), slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	login := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/login", "", map[string]any{"username": "admin", "password": "admin123", "tenantId": "tenant_001"}, http.StatusOK)
	token := login["accessToken"].(string)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("pointTable", "temperature = byte 1 / 10")
	_ = writer.WriteField("samplePayload", "01 2A")
	file, _ := writer.CreateFormFile("file", "protocol.csv")
	_, _ = file.Write([]byte("offset,name\n1,temperature\n"))
	_ = writer.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/ai/protocol-assistant/generate", &body)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var draft model.ProtocolAssistantDraft
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&draft) != nil {
		response.Body.Close()
		t.Fatalf("generate protocol assistant status=%d", response.StatusCode)
	}
	response.Body.Close()
	if draft.Source == "" || len(draft.Fields) != 1 {
		t.Fatalf("unexpected generated draft %#v", draft)
	}
	draft.Fields[0].Name = "temperature_centi"
	draft.Fields[0].Expression = "bytes[1]"
	draft.Source, err = core.BuildProtocolJavaScriptSource(draft)
	if err != nil {
		t.Fatal(err)
	}
	preview := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/ai/protocol-assistant/preview", token, map[string]any{"draft": draft, "source": draft.Source, "payload": "012A", "payloadFormat": "hex"}, http.StatusOK)
	if preview["success"] != true || preview["standardMessage"].(map[string]any)["properties"].(map[string]any)["temperature_centi"] != float64(42) {
		t.Fatalf("unexpected preview %#v", preview)
	}
	published := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/ai/protocol-assistant/publish", token, map[string]any{"draft": draft, "source": draft.Source, "payload": "012A", "payloadFormat": "hex", "status": "PUBLISHED"}, http.StatusCreated)
	pkg := published["package"].(map[string]any)
	if pkg["parserType"] != parser.JavaScriptParserName || pkg["status"] != "PUBLISHED" {
		t.Fatalf("unexpected published package %#v", pkg)
	}
	if !strings.Contains(pkg["config"].(map[string]any)["source"].(string), "temperature_centi") {
		t.Fatalf("published package did not persist edited field: %#v", pkg)
	}
}
