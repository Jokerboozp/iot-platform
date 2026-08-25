// Command go-protocol-worker is a minimal example for the uploaded Go
// protocol-package contract. Replace the payload conversion with the vendor
// protocol implementation and keep the one-request/one-response JSON shape.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type rawMessage struct {
	Payload       json.RawMessage `json:"payload"`
	PayloadFormat string          `json:"payloadFormat"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024), 2<<20)
	if !scanner.Scan() {
		writeError("a RawMessage JSON line is required")
		return
	}
	var raw rawMessage
	if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
		writeError("invalid RawMessage: " + err.Error())
		return
	}
	properties := map[string]any{}
	if strings.EqualFold(raw.PayloadFormat, "hex") {
		text := strings.Trim(strings.TrimSpace(string(raw.Payload)), `"`)
		data, err := hex.DecodeString(strings.NewReplacer(" ", "", "\r", "", "\n", "", "\t", "").Replace(text))
		if err != nil {
			writeError("invalid hex payload: " + err.Error())
			return
		}
		if len(data) > 0 {
			properties["firstByte"] = int(data[0])
		}
		if len(data) > 1 {
			properties["secondByte"] = int(data[1])
		}
	} else {
		var body any
		if err := json.Unmarshal(raw.Payload, &body); err != nil {
			writeError("invalid JSON payload: " + err.Error())
			return
		}
		properties["body"] = body
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"messageType": "PROPERTY_REPORT",
		"properties":  properties,
		"tags":        map[string]string{"worker": "go-example"},
	})
}

func writeError(message string) {
	_, _ = fmt.Fprintf(os.Stdout, `{"error":%q}`+"\n", message)
}
