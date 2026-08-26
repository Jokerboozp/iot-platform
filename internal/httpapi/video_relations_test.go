package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"iot-platform/internal/adapters/local"
	"iot-platform/internal/adapters/memory"
	"iot-platform/internal/config"
	"iot-platform/internal/core"
	"iot-platform/internal/metrics"
	"iot-platform/internal/parser"
)

func TestVideoCameraRelationsHTTPSupportsManyToManyReverseLookup(t *testing.T) {
	repo := memory.NewRepository()
	archive, err := local.NewArchive(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	engine := core.New(repo, archive, local.NewBus(), local.NewRealtime(), parser.NewRegistry(parser.JSONParser{}), slog.New(slog.NewTextHandler(io.Discard, nil)))
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
	login := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/login", "", map[string]any{
		"username": "admin", "password": "admin123", "tenantId": "tenant_001",
	}, http.StatusOK)
	token := login["accessToken"].(string)

	camera := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/integrations/video/cameras", token, map[string]any{
		"cameraId":         "camera_m2m",
		"cameraName":       "多对多摄像头",
		"streamUrl":        "https://media.example.internal/live/m2m.m3u8",
		"relatedDeviceIds": []string{"device-001", "device-002"},
		"relatedFloorIds":  []string{"floor-01", "floor-02"},
		"relatedRoomIds":   []string{"room-101", "room-102"},
		"enabled":          true,
	}, http.StatusCreated)
	if got := len(camera["relatedDeviceIds"].([]any)); got != 2 {
		t.Fatalf("saved camera device relation count = %d, want 2: %#v", got, camera)
	}

	for relationType, targetID := range map[string]string{
		"device": "device-002",
		"floor":  "floor-02",
		"room":   "room-102",
	} {
		result := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/integrations/video/relations?relationType="+relationType+"&targetId="+targetID, token, nil, http.StatusOK)
		items := result["items"].([]any)
		if len(items) != 1 || items[0].(map[string]any)["cameraId"] != "camera_m2m" {
			t.Fatalf("reverse %s lookup = %#v", relationType, result)
		}
	}
}
