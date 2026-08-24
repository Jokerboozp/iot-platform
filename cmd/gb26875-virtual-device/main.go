package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"iot-platform/internal/model"
	"iot-platform/internal/parser"
)

type client struct {
	baseURL string
	token   string
	http    *http.Client
}

func main() {
	var (
		baseURL  = flag.String("platform", "http://localhost:8081", "platform API base URL")
		gateway  = flag.String("gateway", "", "optional GB26875 TCP gateway address, for example localhost:26875")
		network  = flag.String("network", "tcp", "gateway network: tcp or udp")
		tenant   = flag.String("tenant", "tenant_001", "tenant ID")
		username = flag.String("username", "admin", "platform username")
		password = flag.String("password", "admin123", "platform password")
		deviceID = flag.String("device", "gb26875_virtual_001", "virtual device ID")
		source   = flag.String("source", "123456789012", "12 hexadecimal source-address digits")
		scenario = flag.String("scenario", "manual-alarm", "manual-alarm, manual-normal, smoke-alarm, sound-light-start or sound-light-stop")
		dryRun   = flag.Bool("dry-run", false, "only print the generated protocol frame")
	)
	flag.Parse()

	sourceBytes, err := hex.DecodeString(strings.TrimSpace(*source))
	check(err)
	if len(sourceBytes) != 6 {
		check(fmt.Errorf("source must contain exactly 12 hexadecimal digits"))
	}
	var sourceAddress [6]byte
	copy(sourceAddress[:], sourceBytes)
	componentType, status, description, err := scenarioValues(*scenario)
	check(err)
	frame := parser.BuildGB26875ComponentStatusFrame(1, sourceAddress, 128, 1, componentType, 1, 1, status, description, time.Now())
	hexFrame := strings.ToUpper(hex.EncodeToString(frame))
	if *dryRun {
		fmt.Println(hexFrame)
		return
	}
	if *gateway != "" {
		if *network != "tcp" && *network != "udp" {
			check(fmt.Errorf("network must be tcp or udp"))
		}
		conn, err := net.DialTimeout(*network, *gateway, 5*time.Second)
		check(err)
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		registration := parser.BuildGB26875RegistrationFrame(0, sourceAddress, time.Now())
		_, err = conn.Write(registration)
		check(err)
		registrationAck := make([]byte, 30)
		_, err = io.ReadFull(conn, registrationAck)
		check(err)
		_, err = conn.Write(frame)
		check(err)
		ack := make([]byte, 30)
		_, err = io.ReadFull(conn, ack)
		check(err)
		ackPayload, _ := json.Marshal(hex.EncodeToString(ack))
		ackMessage, err := (parser.GB26875Parser{}).Parse(model.RawMessage{MessageID: "raw_ack", Protocol: "gb26875-dahua-v1.03", PayloadFormat: "hex", Payload: ackPayload, ReceivedAt: time.Now().UnixMilli()})
		check(err)
		fmt.Printf("虚拟设备已通过 %s 网关上报 %s，平台返回 %v\n协议帧: %s\n", strings.ToUpper(*network), *scenario, ackMessage.Event["type"], hexFrame)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := &client{baseURL: strings.TrimRight(*baseURL, "/"), http: &http.Client{Timeout: 10 * time.Second}}
	var login struct {
		AccessToken string `json:"accessToken"`
	}
	check(c.do(ctx, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": *username, "password": *password, "tenantId": *tenant}, &login))
	c.token = login.AccessToken
	check(c.do(ctx, http.MethodPost, "/api/v1/protocol-packages", map[string]any{
		"id": "protocol_gb26875_dahua_v103", "name": "国标消防终端协议（大华 v1.03）", "version": "1.0.0",
		"protocol": "gb26875-dahua-v1.03", "transport": "TCP_UDP_BRIDGE", "payloadFormat": "hex",
		"parserType": "gb26875_dahua_parser", "status": "PUBLISHED", "description": "GB/T 26875.3-2011 TCP/UDP 帧及大华消防终端 v1.03 补充协议",
	}, nil))
	check(c.do(ctx, http.MethodPost, "/api/v1/products", map[string]any{
		"id": "product_gb26875_lora_fire", "name": "LoRa 声光与手报", "category": "fire-alarm",
		"protocolPackageId": "protocol_gb26875_dahua_v103", "status": "ENABLED",
		"description": "GB26875 虚拟/真实 LoRa 消防部件",
	}, nil))
	check(c.do(ctx, http.MethodPost, "/api/v1/device-registry", map[string]any{
		"id": *deviceID, "productId": "product_gb26875_lora_fire", "name": "GB26875 虚拟消防设备", "status": "ENABLED",
		"deviceRole": "DIRECT", "registrationSource": "VIRTUAL_DEVICE", "tags": map[string]string{"sourceAddress": strings.ToUpper(*source), "protocol": "GB26875"},
	}, nil))
	var result map[string]any
	check(c.do(ctx, http.MethodPost, "/api/v1/device-registry/"+*deviceID+"/debug", map[string]any{
		"messageId": fmt.Sprintf("raw_gb26875_%d", time.Now().UnixNano()), "payload": hexFrame,
	}, &result))
	encoded, _ := json.MarshalIndent(result, "", "  ")
	fmt.Printf("虚拟设备 %s 已上报场景 %s\n协议帧: %s\n平台响应: %s\n", *deviceID, *scenario, hexFrame, encoded)
}

func scenarioValues(name string) (byte, uint16, string, error) {
	switch name {
	case "manual-alarm":
		return 23, 1 << 1, "virtual manual alarm", nil
	case "manual-normal":
		return 23, 0, "virtual manual normal", nil
	case "smoke-alarm":
		return 40, 1 << 1, "virtual smoke alarm", nil
	case "sound-light-start":
		return 137, 1<<5 | 1<<6, "virtual sound light started", nil
	case "sound-light-stop":
		return 137, 0, "virtual sound light stopped", nil
	default:
		return 0, 0, "", fmt.Errorf("unknown scenario %q", name)
	}
}

func (c *client) do(ctx context.Context, method, path string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s returned %s: %s", method, path, resp.Status, strings.TrimSpace(string(responseBody)))
	}
	if out != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, out); err != nil {
			return err
		}
	}
	return nil
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}
