package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"iot-platform/internal/model"
	"iot-platform/internal/parser"
)

type platformClient struct {
	baseURL, tenant, token string
	http                   *http.Client
	mu                     sync.Mutex
	devices                map[string]bool
}

func main() {
	listen := flag.String("listen", env("GB26875_LISTEN", ":26875"), "TCP listen address")
	udpListen := flag.String("udp-listen", env("GB26875_UDP_LISTEN", ":26875"), "UDP listen address; empty disables UDP")
	platformURL := flag.String("platform", env("GB26875_PLATFORM_URL", "http://localhost:8081"), "platform API base URL")
	tenant := flag.String("tenant", env("GB26875_TENANT", "tenant_001"), "platform tenant")
	username := flag.String("username", env("GB26875_USERNAME", "admin"), "platform username")
	password := flag.String("password", env("GB26875_PASSWORD", "admin123"), "platform password")
	flag.Parse()

	ctx := context.Background()
	c := &platformClient{baseURL: strings.TrimRight(*platformURL, "/"), tenant: *tenant, http: &http.Client{Timeout: 10 * time.Second}, devices: map[string]bool{}}
	if err := c.login(ctx, *username, *password); err != nil {
		log.Fatal(err)
	}
	if err := c.setup(ctx); err != nil {
		log.Fatal(err)
	}
	if *udpListen != "" {
		udpAddress, err := net.ResolveUDPAddr("udp", *udpListen)
		if err != nil {
			log.Fatal(err)
		}
		udpConn, err := net.ListenUDP("udp", udpAddress)
		if err != nil {
			log.Fatal(err)
		}
		defer udpConn.Close()
		go handleUDP(c, udpConn)
		log.Printf("GB26875 UDP gateway listening on %s", *udpListen)
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	log.Printf("GB26875 gateway listening on %s, forwarding to %s", *listen, c.baseURL)
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go handleConnection(c, conn)
	}
}

func handleConnection(c *platformClient, conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		frame, err := readFrame(reader)
		if err != nil {
			if !errors.Is(err, io.EOF) && !isTimeout(err) {
				log.Printf("%s invalid frame: %v", conn.RemoteAddr(), err)
			}
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		ack, deviceID, err := processFrame(ctx, c, frame, "TCP", conn.RemoteAddr().String())
		cancel()
		if err != nil {
			log.Printf("%s forward failed: %v", deviceID, err)
			continue
		}
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := conn.Write(ack); err != nil {
			log.Printf("%s ack failed: %v", deviceID, err)
			return
		}
		log.Printf("%s accepted %d-byte frame and sent confirmation", deviceID, len(frame))
	}
}

func handleUDP(c *platformClient, conn *net.UDPConn) {
	buffer := make([]byte, 2+25+512+3)
	for {
		count, remote, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("udp read: %v", err)
			return
		}
		frame := append([]byte(nil), buffer[:count]...)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		ack, deviceID, err := processFrame(ctx, c, frame, "UDP", remote.String())
		cancel()
		if err != nil {
			log.Printf("%s UDP frame rejected: %v", deviceID, err)
			continue
		}
		if _, err := conn.WriteToUDP(ack, remote); err != nil {
			log.Printf("%s UDP ack failed: %v", deviceID, err)
			continue
		}
		log.Printf("%s accepted %d-byte UDP frame and sent confirmation", deviceID, len(frame))
	}
}

func processFrame(ctx context.Context, c *platformClient, frame []byte, transport, remote string) ([]byte, string, error) {
	if len(frame) < 18 {
		return nil, "unknown", errors.New("frame is too short")
	}
	source := strings.ToUpper(hex.EncodeToString(frame[12:18]))
	deviceID := "gb26875_" + strings.ToLower(source)
	payload, _ := json.Marshal(strings.ToUpper(hex.EncodeToString(frame)))
	raw := model.RawMessage{MessageID: fmt.Sprintf("raw_gb26875_%s_%d", strings.ToLower(source), time.Now().UnixNano()), TenantID: c.tenant, ProductID: "product_gb26875_lora_fire", DeviceID: deviceID, Protocol: "gb26875-dahua-v1.03", Transport: transport, PayloadFormat: "hex", Payload: payload, ReceivedAt: time.Now().UnixMilli(), Source: "gb26875-" + strings.ToLower(transport) + "-gateway", RemoteAddress: remote}
	if _, err := (parser.GB26875Parser{}).Parse(raw); err != nil {
		return nil, deviceID, err
	}
	if err := c.forward(ctx, raw, source); err != nil {
		return nil, deviceID, err
	}
	var destination [6]byte
	copy(destination[:], frame[12:18])
	return parser.BuildGB26875AckFrame(binary.LittleEndian.Uint16(frame[2:4]), destination, time.Now()), deviceID, nil
}

func readFrame(reader *bufio.Reader) ([]byte, error) {
	for {
		first, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		if first != '@' {
			continue
		}
		second, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		if second != '@' {
			continue
		}
		break
	}
	header := make([]byte, 25)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, err
	}
	applicationLength := int(binary.LittleEndian.Uint16(header[22:24]))
	if applicationLength > 512 {
		return nil, fmt.Errorf("application length %d exceeds 512", applicationLength)
	}
	tail := make([]byte, applicationLength+3)
	if _, err := io.ReadFull(reader, tail); err != nil {
		return nil, err
	}
	frame := append([]byte{'@', '@'}, header...)
	return append(frame, tail...), nil
}

func (c *platformClient) login(ctx context.Context, username, password string) error {
	var out struct {
		AccessToken string `json:"accessToken"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": username, "password": password, "tenantId": c.tenant}, &out); err != nil {
		return fmt.Errorf("platform login: %w", err)
	}
	c.token = out.AccessToken
	return nil
}

func (c *platformClient) setup(ctx context.Context) error {
	if err := c.do(ctx, http.MethodPost, "/api/v1/protocol-packages", map[string]any{"id": "protocol_gb26875_dahua_v103", "name": "国标消防终端协议（大华 v1.03）", "version": "1.0.0", "protocol": "gb26875-dahua-v1.03", "transport": "TCP", "payloadFormat": "hex", "parserType": "gb26875_dahua_parser", "status": "PUBLISHED"}, nil); err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, "/api/v1/products", map[string]any{"id": "product_gb26875_lora_fire", "name": "LoRa 声光与手报", "category": "fire-alarm", "protocolPackageId": "protocol_gb26875_dahua_v103", "transport": "TCP", "payloadFormat": "hex", "status": "ENABLED"}, nil)
}

func (c *platformClient) forward(ctx context.Context, raw model.RawMessage, source string) error {
	c.mu.Lock()
	known := c.devices[raw.DeviceID]
	c.mu.Unlock()
	if !known {
		if err := c.do(ctx, http.MethodPost, "/api/v1/device-registry", map[string]any{"id": raw.DeviceID, "productId": raw.ProductID, "name": "GB26875 设备 " + source, "status": "ENABLED", "deviceRole": "DIRECT", "registrationSource": "GB26875_TCP", "tags": map[string]string{"sourceAddress": source}}, nil); err != nil {
			return err
		}
		c.mu.Lock()
		c.devices[raw.DeviceID] = true
		c.mu.Unlock()
	}
	return c.do(ctx, http.MethodPost, "/api/v1/raw-messages", raw, nil)
}

func (c *platformClient) do(ctx context.Context, method, path string, body, out any) error {
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
		return fmt.Errorf("%s %s: %s", resp.Status, path, strings.TrimSpace(string(responseBody)))
	}
	if out != nil && len(responseBody) > 0 {
		return json.Unmarshal(responseBody, out)
	}
	return nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
