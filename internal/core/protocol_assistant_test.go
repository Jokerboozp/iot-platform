package core

import (
	"context"
	"io"
	"log/slog"
	"testing"

	aiadapter "iot-platform/internal/adapters/ai"
	"iot-platform/internal/adapters/local"
	"iot-platform/internal/adapters/memory"
	"iot-platform/internal/model"
	"iot-platform/internal/parser"
)

type protocolAssistantAI struct{}

func (protocolAssistantAI) AnalyzeAlarm(context.Context, model.Alarm, []map[string]any, []string) (model.AIAnalysis, error) {
	return model.AIAnalysis{}, nil
}
func (protocolAssistantAI) Chat(context.Context, string, string) (string, error) {
	return "巡检建议", nil
}
func (protocolAssistantAI) RuleDraft(context.Context, string, string) (model.AlarmRule, error) {
	return model.AlarmRule{}, nil
}
func (protocolAssistantAI) Health(context.Context) error { return nil }
func (protocolAssistantAI) GenerateJSON(context.Context, string, string, string) (string, error) {
	return `{"name":"测试十六进制协议","protocol":"test-hex","transport":"TCP","payloadFormat":"hex","messageType":"PROPERTY_REPORT","setup":"const bytes = hexToBytes(raw.payload)","fields":[{"name":"temperature","label":"温度","type":"number","expression":"bytes[1] / 10"},{"name":"smoke","label":"烟雾","type":"boolean","expression":"(bytes[0] & 1) === 1"}]}`, nil
}

func TestProtocolAssistantBuildAndPreview(t *testing.T) {
	draft := model.ProtocolAssistantDraft{PayloadFormat: "hex", MessageType: model.PropertyReport, Setup: "const bytes = hexToBytes(raw.payload)", Fields: []model.ProtocolAssistantField{{Name: "temperature", Expression: "bytes[1] / 10"}, {Name: "smoke", Expression: "(bytes[0] & 1) === 1"}}}
	source, err := BuildProtocolJavaScriptSource(draft)
	if err != nil {
		t.Fatal(err)
	}
	draft.Source = source
	message, err := PreviewProtocolAssistant(draft, "tenant-test", "01 2A")
	if err != nil {
		t.Fatal(err)
	}
	if message.Properties["temperature"] != 4.2 || message.Properties["smoke"] != true || message.Parser != parser.JavaScriptParserName {
		t.Fatalf("unexpected protocol preview %#v", message)
	}
}

func TestProtocolAssistantRejectsUnsafeExpression(t *testing.T) {
	_, err := BuildProtocolJavaScriptSource(model.ProtocolAssistantDraft{PayloadFormat: "hex", Fields: []model.ProtocolAssistantField{{Name: "x", Expression: "fetch('http://example')"}}})
	if err == nil {
		t.Fatal("unsafe expression was accepted")
	}
}

func TestGenerateProtocolAssistant(t *testing.T) {
	repo := memory.NewRepository()
	archive, err := local.NewArchive(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	engine := New(repo, archive, local.NewBus(), local.NewRealtime(), parser.NewRegistry(parser.JavaScriptParser{}), slog.New(slog.NewTextHandler(io.Discard, nil)))
	engine.AI = protocolAssistantAI{}
	draft, err := engine.GenerateProtocolAssistant(context.Background(), "tenant-test", ProtocolAssistantInput{PointTable: "温度：第 2 字节，单位 0.1 度", SamplePayload: "01 2A"})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Source == "" || len(draft.Fields) != 2 || draft.Preview == nil || draft.Preview.Properties["temperature"] != 4.2 {
		t.Fatalf("unexpected generated draft %#v", draft)
	}
}

func TestInspectDeviceHealth(t *testing.T) {
	repo := memory.NewRepository()
	archive, err := local.NewArchive(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	engine := New(repo, archive, local.NewBus(), local.NewRealtime(), parser.NewRegistry(parser.JSONParser{}), slog.New(slog.NewTextHandler(io.Discard, nil)))
	engine.AI = aiadapter.NoopAI{}
	ctx := context.Background()
	if err = repo.SaveManagedDevice(ctx, model.ManagedDevice{ID: "device-health", TenantID: "tenant-test", ProductID: "sensor", Name: "测试设备"}); err != nil {
		t.Fatal(err)
	}
	if err = repo.UpsertDeviceState(ctx, model.DeviceState{TenantID: "tenant-test", ProductID: "sensor", DeviceID: "device-health", BusinessStatus: "OFFLINE", DataStatus: "SILENT", LastSeenAt: 1}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = repo.UpsertAlarm(ctx, model.Alarm{ID: "alarm-health", TenantID: "tenant-test", DeviceID: "device-health", Status: "ACTIVE", AlarmLevel: "HIGH"}); err != nil {
		t.Fatal(err)
	}
	report, err := engine.InspectDeviceHealth(ctx, "tenant-test")
	if err != nil {
		t.Fatal(err)
	}
	if report.Counts["total"] != 1 || report.Counts["offline"] != 1 || report.Items[0].ActiveAlarmCount != 1 {
		t.Fatalf("unexpected health report %#v", report)
	}
}
