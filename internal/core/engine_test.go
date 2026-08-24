package core

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	aiadapter "iot-platform/internal/adapters/ai"
	"iot-platform/internal/adapters/knowledge"
	"iot-platform/internal/adapters/local"
	"iot-platform/internal/adapters/memory"
	"iot-platform/internal/model"
	"iot-platform/internal/parser"
	"iot-platform/internal/ports"
)

func TestRawToAlarmPipeline(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	archive, err := local.NewArchive(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bus := local.NewBus()
	realtime := local.NewRealtime()
	e := New(repo, archive, bus, realtime, parser.NewRegistry(parser.JSONParser{}), slog.New(slog.NewTextHandler(io.Discard, nil)))
	e.AI = aiadapter.NoopAI{}
	e.KB = knowledge.NewLocal()
	if err = e.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err = repo.SaveVideoCameraMapping(ctx, model.VideoCameraMapping{TenantID: "t1", CameraID: "camera-001", CameraName: "一号摄像头", StreamURL: "https://media.example/live.m3u8", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	rule := model.AlarmRule{ID: "r1", TenantID: "t1", Name: "高温烟雾", AlarmType: "FIRE", Level: "HIGH", Enabled: true, Conditions: []model.RuleCondition{{Field: "temperature", Operator: ">", Value: 80}, {Field: "smoke", Operator: "eq", Value: true}}, Actions: []model.RuleAction{{Type: "OPEN_CAMERA", CameraID: "camera-001"}}}
	if err = repo.SaveRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	raw := model.RawMessage{MessageID: "raw_1", TenantID: "t1", ProductID: "json_sensor", DeviceID: "device_1", Protocol: "json", PayloadFormat: "json", ReceivedAt: time.Now().UnixMilli(), Payload: json.RawMessage(`{"properties":{"temperature":90,"smoke":true},"tags":{"cityCode":"city","districtCode":"d","buildingId":"b","deviceType":"smoke"}}`)}
	if _, created, err := e.IngestRaw(ctx, raw); err != nil || !created {
		t.Fatalf("ingest created=%v err=%v", created, err)
	}
	alarms, err := repo.ListAlarms(ctx, ports.AlarmFilter{TenantID: "t1", Status: "ACTIVE"})
	if err != nil || len(alarms) != 1 {
		t.Fatalf("alarms=%v err=%v", alarms, err)
	}
	if alarms[0].TriggerCount != 1 {
		t.Fatalf("unexpected alarm %#v", alarms[0])
	}
	if _, created, err := e.IngestRaw(ctx, raw); err != nil || created {
		t.Fatalf("duplicate created=%v err=%v", created, err)
	}
	if _, err = repo.GetAIAnalysis(ctx, alarms[0].TenantID, alarms[0].ID); err != nil {
		t.Fatalf("ai analysis not saved: %v", err)
	}
	if len(realtime.Messages) < 2 {
		t.Fatalf("expected state and alarm realtime messages, got %d", len(realtime.Messages))
	}
	foundAction := false
	for _, published := range realtime.Messages {
		if published.Topic != "/iot/ui-action/t1" {
			continue
		}
		var event model.UIActionEvent
		if err = json.Unmarshal(published.Payload, &event); err != nil {
			t.Fatal(err)
		}
		foundAction = event.RuleID == "r1" && event.Action.Type == "OPEN_CAMERA" && event.Action.CameraID == "camera-001"
	}
	if !foundAction {
		t.Fatalf("expected validated UI action event, messages=%#v", realtime.Messages)
	}
}

func TestGatewayAutomaticallyRegistersChildDevice(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	archive, err := local.NewArchive(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := New(repo, archive, local.NewBus(), local.NewRealtime(), parser.NewRegistry(parser.JSONParser{}), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err = e.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err = repo.SaveProduct(ctx, model.Product{ID: "gateway_product", TenantID: "t1", Name: "网关产品", Category: "gateway", Status: "ENABLED"}); err != nil {
		t.Fatal(err)
	}
	if err = repo.SaveProduct(ctx, model.Product{ID: "sensor_product", TenantID: "t1", Name: "传感器产品", Category: "sensor", Status: "ENABLED"}); err != nil {
		t.Fatal(err)
	}
	if err = repo.SaveManagedDevice(ctx, model.ManagedDevice{ID: "gateway_1", TenantID: "t1", ProductID: "gateway_product", Name: "一号网关", Status: "ENABLED", DeviceRole: "GATEWAY", AccessKey: "dk_gateway", SecretHash: "unused"}); err != nil {
		t.Fatal(err)
	}
	raw := model.RawMessage{MessageID: "raw_child_1", TenantID: "t1", ProductID: "sensor_product", DeviceID: "child_1", DeviceName: "一号烟感", GatewayID: "gateway_1", Protocol: "json", PayloadFormat: "json", Payload: json.RawMessage(`{"properties":{"temperature":25.5}}`)}
	if _, created, ingestErr := e.IngestRaw(ctx, raw); ingestErr != nil || !created {
		t.Fatalf("ingest created=%v err=%v", created, ingestErr)
	}
	child, err := repo.GetManagedDevice(ctx, "t1", "child_1")
	if err != nil {
		t.Fatal(err)
	}
	if child.DeviceRole != "CHILD" || child.GatewayID != "gateway_1" || !child.AutoRegistered || child.RegistrationSource != "GATEWAY_AUTO" || child.ProductID != "sensor_product" {
		t.Fatalf("unexpected child registration %#v", child)
	}
}

func TestStateChangeIsStoredAsStandardMessage(t *testing.T) {
	repo := memory.NewRepository()
	archive, err := local.NewArchive(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bus := local.NewBus()
	e := New(repo, archive, bus, local.NewRealtime(), parser.NewRegistry(parser.JSONParser{}), slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := e.Start(ctx); err != nil {
		t.Fatal(err)
	}
	raw := model.RawMessage{MessageID: "raw_state_change", TenantID: "t1", ProductID: "json_sensor", DeviceID: "state_device", Protocol: "json", PayloadFormat: "json", Payload: json.RawMessage(`{"businessStatus":"ONLINE"}`)}
	if _, _, err := e.IngestRaw(ctx, raw); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if message, getErr := repo.GetStandardMessageByRaw(ctx, "t1", raw.MessageID); getErr == nil {
			if message.MessageType != model.StateChange {
				t.Fatalf("unexpected message type: %s", message.MessageType)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("state-change standard message was not stored")
}
