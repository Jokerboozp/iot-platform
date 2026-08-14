package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	aiadapter "iot-platform/internal/adapters/ai"
	"iot-platform/internal/adapters/knowledge"
	"iot-platform/internal/adapters/local"
	"iot-platform/internal/adapters/memory"
	"iot-platform/internal/config"
	"iot-platform/internal/core"
	"iot-platform/internal/metrics"
	"iot-platform/internal/parser"
)

func TestHTTPWorkflow(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	archive, err := local.NewArchive(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	engine := core.New(repo, archive, local.NewBus(), local.NewRealtime(), parser.NewRegistry(parser.JSONParser{}), slog.New(slog.NewTextHandler(io.Discard, nil)))
	engine.AI = aiadapter.NoopAI{}
	engine.KB = knowledge.NewLocal()
	engine.Metrics = metrics.New()
	if err = engine.Start(ctx); err != nil {
		t.Fatal(err)
	}
	cfg := config.Load()
	cfg.DevMode = true
	cfg.JWTSecret = "test-secret-at-least-32-characters"
	server := httptest.NewServer(New(cfg, engine, engine.Metrics.(*metrics.Registry), slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	defer server.Close()
	login := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/login", "", map[string]any{"username": "admin", "password": "admin123", "tenantId": "tenant_001"}, 200)
	token := login["accessToken"].(string)
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/protocol-packages", token, map[string]any{"id": "protocol_json", "name": "JSON 通用协议", "version": "1.0.0", "protocol": "json", "transport": "HTTP", "payloadFormat": "json", "parserType": "custom_json_parser", "status": "PUBLISHED"}, 201)
	protocolTest := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/protocol-packages/protocol_json/test", token, map[string]any{"payload": map[string]any{"properties": map[string]any{"temperature": 22.5}}}, 200)
	if protocolTest["success"] != true {
		t.Fatalf("protocol test failed: %#v", protocolTest)
	}
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/products", token, map[string]any{"id": "product_json", "name": "JSON 传感器", "category": "sensor", "protocolPackageId": "protocol_json", "status": "ENABLED"}, 201)
	managed := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/device-registry", token, map[string]any{"id": "device_managed", "name": "受管测试设备", "productId": "product_json", "status": "ENABLED"}, 201)
	credential := managed["credential"].(map[string]any)
	publicBody, _ := json.Marshal(map[string]any{"messageId": "raw_managed", "payload": map[string]any{"properties": map[string]any{"temperature": 22.5}}})
	publicReq, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/device-ingest/device_managed", bytes.NewReader(publicBody))
	publicReq.Header.Set("Content-Type", "application/json")
	publicReq.Header.Set("X-Device-Key", credential["accessKey"].(string))
	publicReq.Header.Set("X-Device-Secret", credential["secret"].(string))
	publicResp, err := server.Client().Do(publicReq)
	if err != nil || publicResp.StatusCode != 201 {
		t.Fatalf("managed ingest status=%v err=%v", publicResp, err)
	}
	publicResp.Body.Close()
	registry := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/device-registry", token, nil, 200)
	if registry["count"].(float64) != 1 {
		t.Fatalf("unexpected registry: %#v", registry)
	}
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/rules", token, map[string]any{"id": "rule_e2e", "name": "高温烟雾", "alarmType": "FIRE_RISK", "level": "HIGH", "match": "all", "enabled": true, "conditions": []map[string]any{{"field": "temperature", "operator": ">", "value": 80}, {"field": "smoke", "operator": "eq", "value": true}}}, 201)
	now := time.Now().UnixMilli()
	ingest := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/raw-messages", token, map[string]any{"messageId": "raw_http_e2e", "tenantId": "tenant_001", "productId": "json_smoke", "deviceId": "device_http_e2e", "protocol": "json", "payloadFormat": "json", "receivedAt": now, "payload": map[string]any{"properties": map[string]any{"temperature": 88.5, "smoke": true}, "tags": map[string]any{"cityCode": "city_001", "districtCode": "district_01", "buildingId": "A", "deviceType": "smoke"}}}, 201)
	if ingest["created"] != true {
		t.Fatalf("raw not created: %#v", ingest)
	}
	alarms := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/alarms?status=ACTIVE", token, nil, 200)
	if alarms["count"].(float64) != 1 {
		t.Fatalf("unexpected alarms %#v", alarms)
	}
	items := alarms["items"].([]any)
	alarmID := items[0].(map[string]any)["alarmId"].(string)
	rawDetail := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/raw-messages/raw_http_e2e", token, nil, 200)
	if rawDetail["message"].(map[string]any)["messageId"] != "raw_http_e2e" {
		t.Fatalf("unexpected raw detail %#v", rawDetail)
	}
	downloadReq, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/raw-messages/raw_http_e2e/download", nil)
	downloadReq.Header.Set("Authorization", "Bearer "+token)
	downloadResp, err := server.Client().Do(downloadReq)
	if err != nil {
		t.Fatal(err)
	}
	downloadBody, _ := io.ReadAll(downloadResp.Body)
	downloadResp.Body.Close()
	if downloadResp.StatusCode != 200 || !strings.Contains(downloadResp.Header.Get("Content-Disposition"), "raw_http_e2e.json") || !bytes.Contains(downloadBody, []byte(`"messageId": "raw_http_e2e"`)) {
		t.Fatalf("raw download status=%d disposition=%q body=%s", downloadResp.StatusCode, downloadResp.Header.Get("Content-Disposition"), downloadBody)
	}
	batchRequestBody, _ := json.Marshal(map[string]any{"messageIds": []string{"raw_managed", "raw_http_e2e"}})
	batchReq, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/raw-messages/download", bytes.NewReader(batchRequestBody))
	batchReq.Header.Set("Authorization", "Bearer "+token)
	batchReq.Header.Set("Content-Type", "application/json")
	batchResp, err := server.Client().Do(batchReq)
	if err != nil {
		t.Fatal(err)
	}
	batchBody, _ := io.ReadAll(batchResp.Body)
	batchResp.Body.Close()
	if batchResp.StatusCode != 200 || batchResp.Header.Get("Content-Type") != "application/zip" || batchResp.Header.Get("X-Archive-Count") != "2" {
		t.Fatalf("batch raw download status=%d type=%q count=%q body=%s", batchResp.StatusCode, batchResp.Header.Get("Content-Type"), batchResp.Header.Get("X-Archive-Count"), batchBody)
	}
	zr, err := zip.NewReader(bytes.NewReader(batchBody), int64(len(batchBody)))
	if err != nil {
		t.Fatal(err)
	}
	zipNames := map[string]bool{}
	for _, file := range zr.File {
		zipNames[file.Name] = true
	}
	if !zipNames["清单.json"] || !zipNames["报文/001_raw_managed.json"] || !zipNames["报文/002_raw_http_e2e.json"] {
		t.Fatalf("unexpected ZIP entries %#v", zipNames)
	}
	updatedRule := requestJSON(t, server.Client(), http.MethodPut, server.URL+"/api/v1/rules/rule_e2e", token, map[string]any{"name": "更新后的高温规则", "alarmType": "FIRE_RISK", "level": "CRITICAL", "match": "all", "enabled": true, "conditions": []map[string]any{{"field": "temperature", "operator": ">", "value": 90}}}, 200)
	if updatedRule["id"] != "rule_e2e" || updatedRule["name"] != "更新后的高温规则" {
		t.Fatalf("rule not updated %#v", updatedRule)
	}
	requestJSON(t, server.Client(), http.MethodDelete, server.URL+"/api/v1/rules/rule_e2e", token, nil, 200)
	rules := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/rules", token, nil, 200)
	if len(rules["items"].([]any)) != 0 {
		t.Fatalf("rule not deleted %#v", rules)
	}
	requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/ai/alarm-analysis/"+alarmID, token, nil, 200)
	devices := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/devices", token, nil, 200)
	if devices["online"].(float64) != 2 {
		t.Fatalf("unexpected devices %#v", devices)
	}
	video := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/integrations/video/alarm", "", map[string]any{"eventId": "video_http_e2e", "source": "video", "tenantId": "tenant_001", "cameraId": "camera_1", "alarmType": "FLAME_DETECTED", "alarmLevel": "HIGH", "confidence": 0.92, "eventTime": now, "cityCode": "city_001", "districtCode": "district_01", "buildingId": "A"}, 201)
	if video["created"] != true {
		t.Fatalf("video not created %#v", video)
	}
	replay := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/raw-messages/replay", token, map[string]any{"start": now - 1000, "end": now + 1000, "mode": "DRY_RUN", "ratePerSecond": 1000}, 202)
	replayID := replay["id"].(string)
	deadline := time.Now().Add(2 * time.Second)
	for {
		state := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/replays/"+replayID, token, nil, 200)
		if state["status"] == "COMPLETED" {
			if state["processed"].(float64) != 2 {
				t.Fatalf("unexpected replay %#v", state)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("replay timeout %#v", state)
		}
		time.Sleep(10 * time.Millisecond)
	}
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/raw-messages", token, map[string]any{"messageId": "raw_discovered_e2e", "tenantId": "tenant_001", "productId": "product_json", "deviceId": "discovered_1", "protocol": "json", "payloadFormat": "json", "payload": map[string]any{"properties": map[string]any{"temperature": 23.5}}}, 201)
	discovered := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/discovered-devices/discovered_1/register", token, map[string]any{}, 201)
	if discovered["device"].(map[string]any)["deviceRole"] != "DIRECT" {
		t.Fatalf("unexpected discovered registration %#v", discovered)
	}
	gateway := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/device-registry", token, map[string]any{"id": "gateway_1", "name": "一号网关", "productId": "product_json", "status": "ENABLED", "deviceRole": "GATEWAY"}, 201)
	gatewayCredential := gateway["credential"].(map[string]any)
	childBody, _ := json.Marshal(map[string]any{"messageId": "raw_gateway_child_e2e", "deviceId": "child_1", "deviceName": "一号子设备", "productId": "product_json", "payload": map[string]any{"properties": map[string]any{"temperature": 24.5}}})
	childReq, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/device-ingest/gateway_1", bytes.NewReader(childBody))
	childReq.Header.Set("Content-Type", "application/json")
	childReq.Header.Set("X-Device-Key", gatewayCredential["accessKey"].(string))
	childReq.Header.Set("X-Device-Secret", gatewayCredential["secret"].(string))
	childResp, err := server.Client().Do(childReq)
	if err != nil {
		t.Fatal(err)
	}
	childResp.Body.Close()
	if childResp.StatusCode != 201 {
		t.Fatalf("gateway child ingest status=%d", childResp.StatusCode)
	}
	registryAfterGateway := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/device-registry", token, nil, 200)
	foundChild, gatewayChildCount := false, float64(0)
	for _, item := range registryAfterGateway["items"].([]any) {
		row := item.(map[string]any)
		device := row["device"].(map[string]any)
		if device["id"] == "child_1" {
			foundChild = device["deviceRole"] == "CHILD" && device["gatewayId"] == "gateway_1" && device["autoRegistered"] == true
		}
		if device["id"] == "gateway_1" {
			gatewayChildCount = row["childCount"].(float64)
		}
	}
	if !foundChild || gatewayChildCount != 1 {
		t.Fatalf("gateway relation missing %#v", registryAfterGateway)
	}
	resp, err := server.Client().Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "城市消防 IoT 智能平台") {
		t.Fatalf("frontend failed status=%d", resp.StatusCode)
	}
}

func requestJSON(t *testing.T, client *http.Client, method, url, token string, body any, want int) map[string]any {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	if resp.StatusCode != want {
		t.Fatalf("%s %s status=%d want=%d body=%#v", method, url, resp.StatusCode, want, out)
	}
	return out
}
