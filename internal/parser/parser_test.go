package parser

import (
	"encoding/json"
	"testing"

	"iot-platform/internal/model"
)

func TestJSONParser(t *testing.T) {
	r := NewRegistry(JSONParser{})
	m, err := r.Parse(model.RawMessage{MessageID: "raw_1", TenantID: "t", ProductID: "json_sensor", DeviceID: "d", Protocol: "json", PayloadFormat: "json", ReceivedAt: 1000, Payload: json.RawMessage(`{"properties":{"temperature":82.5,"smoke":true},"tags":{"buildingId":"A"}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if m.Properties["temperature"] != 82.5 || m.Tags["buildingId"] != "A" {
		t.Fatalf("unexpected message: %#v", m)
	}
}

func TestSmokeHexParser(t *testing.T) {
	r := NewRegistry(FireSmokeHexParser{})
	m, err := r.Parse(model.RawMessage{MessageID: "raw_2", TenantID: "t", ProductID: "fire_smoke", DeviceID: "d", PayloadFormat: "hex", Payload: json.RawMessage(`"0102D050"`)})
	if err != nil {
		t.Fatal(err)
	}
	if m.MessageType != model.AlarmReport || m.Properties["temperature"].(float64) != 72 {
		t.Fatalf("unexpected message: %#v", m)
	}
}
