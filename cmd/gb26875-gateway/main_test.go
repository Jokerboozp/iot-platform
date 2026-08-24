package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"iot-platform/internal/model"
	"iot-platform/internal/parser"
)

func TestReadFramePreservesCompleteGB26875Message(t *testing.T) {
	want := parser.BuildGB26875ComponentStatusFrame(9, [6]byte{0x12, 0x34, 0x56, 0x78, 0x90, 0x12}, 128, 1, 23, 1, 2, 1<<1, "gateway test", time.Now())
	got, err := readFrame(bufio.NewReader(bytes.NewReader(append([]byte("noise"), want...))))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("frame changed in gateway\nwant %X\n got %X", want, got)
	}
}

func TestGatewayConfirmationFrameIsParseable(t *testing.T) {
	ack := parser.BuildGB26875AckFrame(9, [6]byte{0x12, 0x34, 0x56, 0x78, 0x90, 0x12}, time.Now())
	payload, _ := json.Marshal(hex.EncodeToString(ack))
	message, err := (parser.GB26875Parser{}).Parse(model.RawMessage{Protocol: "gb26875-dahua-v1.03", PayloadFormat: "hex", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if message.MessageType != model.CommandReply || message.Event["type"] != "ACK" {
		t.Fatalf("unexpected confirmation: %#v", message)
	}
}
