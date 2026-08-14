package mqttadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"iot-platform/internal/model"
)

type Client struct{ client mqtt.Client }

func New(broker, user, password, clientID string) (*Client, error) {
	return NewWithCredentials(broker, clientID, func() (string, string) { return user, password })
}

func NewWithCredentials(broker, clientID string, credentials mqtt.CredentialsProvider) (*Client, error) {
	opts := mqtt.NewClientOptions().AddBroker(broker).SetClientID(clientID).SetCredentialsProvider(credentials).SetConnectRetry(true).SetConnectRetryInterval(3 * time.Second).SetAutoReconnect(true).SetOrderMatters(false)
	c := mqtt.NewClient(opts)
	token := c.Connect()
	if !token.WaitTimeout(10 * time.Second) {
		return nil, fmt.Errorf("mqtt connect timeout")
	}
	if token.Error() != nil {
		return nil, token.Error()
	}
	return &Client{client: c}, nil
}
func (c *Client) Publish(ctx context.Context, topic string, payload []byte, qos byte, retained bool) error {
	token := c.client.Publish(topic, qos, retained, payload)
	done := make(chan struct{})
	go func() { token.Wait(); close(done) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return token.Error()
	}
}
func (c *Client) SubscribeRaw(handler func(context.Context, model.RawMessage) error) error {
	token := c.client.SubscribeMultiple(map[string]byte{"/external/raw/#": 1, "/jetlinks/raw/#": 1}, func(_ mqtt.Client, m mqtt.Message) {
		var raw model.RawMessage
		if err := json.Unmarshal(m.Payload(), &raw); err == nil {
			parts := strings.Split(strings.Trim(m.Topic(), "/"), "/")
			if len(parts) >= 5 && parts[1] == "raw" {
				raw.TenantID, raw.ProductID, raw.DeviceID = parts[2], parts[3], parts[4]
			}
			if raw.Source == "" {
				raw.Source = "external-mqtt"
			}
			go func() { _ = handler(context.Background(), raw) }()
		}
	})
	token.Wait()
	return token.Error()
}
func (c *Client) SubscribeDeviceState(handler func(context.Context, model.DeviceState) error) error {
	token := c.client.Subscribe("/iot/device/state/#", 1, func(_ mqtt.Client, m mqtt.Message) {
		var state model.DeviceState
		if err := json.Unmarshal(m.Payload(), &state); err == nil {
			go func() { _ = handler(context.Background(), state) }()
		}
	})
	token.Wait()
	return token.Error()
}
func (c *Client) SubscribeVideo(handler func(context.Context, model.VideoAlarmEvent) error) error {
	token := c.client.Subscribe("/external/video/alarm/#", 1, func(_ mqtt.Client, m mqtt.Message) {
		var v model.VideoAlarmEvent
		if err := json.Unmarshal(m.Payload(), &v); err == nil {
			go func() { _, _, _ = handlerResult(handler, context.Background(), v) }()
		}
	})
	token.Wait()
	return token.Error()
}
func handlerResult(h func(context.Context, model.VideoAlarmEvent) error, ctx context.Context, v model.VideoAlarmEvent) (bool, bool, error) {
	return true, true, h(ctx, v)
}
func (c *Client) Health(context.Context) error {
	if !c.client.IsConnected() {
		return fmt.Errorf("mqtt disconnected")
	}
	return nil
}
func (c *Client) Close() error { c.client.Disconnect(250); return nil }
