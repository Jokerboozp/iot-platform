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
	"path/filepath"
	"strings"
	"testing"

	"iot-platform/internal/adapters/local"
	"iot-platform/internal/adapters/memory"
	"iot-platform/internal/config"
	"iot-platform/internal/core"
	"iot-platform/internal/metrics"
	"iot-platform/internal/parser"
)

func TestUploadGoProtocolArtifactStoresRelativeChecksumMetadata(t *testing.T) {
	repo := memory.NewRepository()
	archive, err := local.NewArchive(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Load()
	cfg.DataDir = t.TempDir()
	cfg.DevMode = true
	cfg.JWTSecret = "test-secret-at-least-32-characters"
	engine := core.New(repo, archive, local.NewBus(), local.NewRealtime(), parser.NewRegistry(parser.ExternalParser{Root: cfg.DataDir}), slog.New(slog.NewTextHandler(io.Discard, nil)))
	engine.Metrics = metrics.New()
	if err := engine.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	api := New(cfg, engine, engine.Metrics.(*metrics.Registry), slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	login := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/login", "", map[string]any{"username": "admin", "password": "admin123", "tenantId": "tenant_001"}, http.StatusOK)
	token := login["accessToken"].(string)
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/protocol-packages", token, map[string]any{
		"id": "protocol_go", "name": "Go Worker", "parserType": parser.GoProtocolParserName, "protocol": "vendor-v2", "transport": "TCP", "payloadFormat": "hex", "status": "DRAFT",
	}, http.StatusCreated)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("artifact", "vendor-worker")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("compiled-worker-placeholder"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/protocol-packages/protocol_go/artifact", &body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		var detail map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&detail)
		t.Fatalf("upload status=%d response=%v", resp.StatusCode, detail)
	}
	var out struct {
		Config map[string]any `json:"config"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	artifact, ok := out.Config["artifact"].(map[string]any)
	if !ok {
		t.Fatalf("missing artifact metadata: %#v", out.Config)
	}
	path, _ := artifact["path"].(string)
	if filepath.IsAbs(path) || !strings.HasPrefix(filepath.ToSlash(path), "protocol-packages/tenant_001/protocol_go/1.0.0/") {
		t.Fatalf("artifact path is not relative to data dir: %q", path)
	}
	if artifact["sha256"] == "" || artifact["protocol"] != "json-lines-v1" {
		t.Fatalf("unexpected artifact metadata: %#v", artifact)
	}
}
