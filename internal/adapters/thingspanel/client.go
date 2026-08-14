package thingspanel

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"iot-platform/internal/model"
	"iot-platform/internal/ports"
)

type Client struct {
	base, user, password string
	repo                 ports.Repository
	http                 *http.Client
	mu                   sync.Mutex
	token                string
	auth                 model.ExternalAuth
}

func New(base, user, password string, repo ports.Repository) *Client {
	return &Client{base: strings.TrimRight(base, "/"), user: user, password: password, repo: repo, http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Authenticate(ctx context.Context, user, password string) (model.ExternalAuth, error) {
	body, _ := json.Marshal(map[string]string{"email": user, "password": password})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/v1/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return model.ExternalAuth{}, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode/100 != 2 {
		return model.ExternalAuth{}, fmt.Errorf("ThingsPanel login %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var envelope map[string]any
	if err = json.Unmarshal(data, &envelope); err != nil {
		return model.ExternalAuth{}, err
	}
	payload := envelope
	if d, ok := envelope["data"].(map[string]any); ok {
		payload = d
	}
	token, _ := payload["token"].(string)
	if token == "" {
		return model.ExternalAuth{}, fmt.Errorf("ThingsPanel login response has no token")
	}
	claims, err := jwtPayload(token)
	if err != nil {
		return model.ExternalAuth{}, err
	}
	tenant := stringValue(claims, "tenant_id", "tenantId")
	if tenant == "" {
		return model.ExternalAuth{}, fmt.Errorf("ThingsPanel token has no tenant_id")
	}
	role := stringValue(claims, "authority", "role")
	if role == "" {
		role = "operator"
	}
	if strings.Contains(strings.ToLower(role), "admin") {
		role = "admin"
	} else {
		role = "operator"
	}
	return model.ExternalAuth{Username: user, TenantID: tenant, Role: role, UpstreamToken: token}, nil
}
func (c *Client) serviceAuth(ctx context.Context) (model.ExternalAuth, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" {
		return c.auth, nil
	}
	auth, err := c.Authenticate(ctx, c.user, c.password)
	if err == nil {
		c.token = auth.UpstreamToken
		c.auth = auth
	}
	return auth, err
}
func (c *Client) Sync(ctx context.Context, tenant string) (model.CatalogSyncResult, error) {
	auth, err := c.serviceAuth(ctx)
	if err != nil {
		return model.CatalogSyncResult{}, err
	}
	if tenant == "" {
		tenant = auth.TenantID
	}
	result := model.CatalogSyncResult{SyncedAt: time.Now().UnixMilli()}
	protocolID := "thingspanel_json"
	_ = c.repo.SaveProtocolPackage(ctx, model.ProtocolPackage{ID: protocolID, TenantID: tenant, Name: "ThingsPanel JSON 标准协议", Version: "1.0.0", Protocol: "json", Transport: "MQTT", PayloadFormat: "json", ParserType: "custom_json_parser", Status: "PUBLISHED", Description: "ThingsPanel 目录同步生成", CreatedAt: result.SyncedAt, UpdatedAt: result.SyncedAt})
	for page := 1; ; page++ {
		devices, total, fetchErr := c.fetchDevices(ctx, auth.UpstreamToken, page)
		if fetchErr != nil {
			return result, fetchErr
		}
		for _, d := range devices {
			productID := d.DeviceConfigID
			if productID == "" {
				productID = "thingspanel_default"
			}
			if _, getErr := c.repo.GetProduct(ctx, tenant, productID); getErr != nil {
				_ = c.repo.SaveProduct(ctx, model.Product{ID: productID, TenantID: tenant, Name: first(d.DeviceConfigName, "ThingsPanel 产品 "+productID), Category: category(d.DeviceType), ProtocolPackageID: protocolID, Transport: "MQTT", PayloadFormat: "json", Status: "ENABLED", Description: "ThingsPanel 同步产品", Metadata: map[string]any{"thingsPanelDeviceConfigId": d.DeviceConfigID}, CreatedAt: result.SyncedAt, UpdatedAt: result.SyncedAt})
				result.Products++
			}
			managed, managedErr := c.repo.GetManagedDevice(ctx, tenant, d.ID)
			if managedErr != nil {
				sum := sha256.Sum256([]byte(c.base + ":" + d.ID))
				managed = model.ManagedDevice{ID: d.ID, TenantID: tenant, AccessKey: "tp_" + hex.EncodeToString(sum[:8]), SecretHash: hex.EncodeToString(sum[:]), SecretHint: "ThingsPanel 托管", CreatedAt: result.SyncedAt, RegistrationSource: "THINGSPANEL_SYNC"}
			}
			managed.ProductID = productID
			managed.Name = first(d.Name, d.DeviceNumber)
			managed.Status = map[bool]string{true: "ENABLED", false: "DISABLED"}[d.DeviceStatus != 0 || d.ActivateFlag == "A"]
			managed.DeviceRole = role(d.DeviceType)
			managed.UpdatedAt = result.SyncedAt
			if saveErr := c.repo.SaveManagedDevice(ctx, managed); saveErr != nil {
				result.Errors = append(result.Errors, saveErr.Error())
			} else {
				result.Devices++
			}
		}
		if page*1000 >= total || len(devices) == 0 {
			break
		}
	}
	return result, nil
}

type deviceRow struct {
	ID               string `json:"id"`
	DeviceNumber     string `json:"device_number"`
	Name             string `json:"name"`
	DeviceConfigID   string `json:"device_config_id"`
	DeviceConfigName string `json:"device_config_name"`
	ActivateFlag     string `json:"activate_flag"`
	DeviceStatus     int    `json:"device_status"`
	DeviceType       string `json:"device_type"`
}

func (c *Client) fetchDevices(ctx context.Context, token string, page int) ([]deviceRow, int, error) {
	u, _ := url.Parse(c.base + "/api/v1/device")
	q := u.Query()
	q.Set("page", fmt.Sprint(page))
	q.Set("page_size", "1000")
	u.RawQuery = q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	// ThingsPanel's authenticated API middleware expects the JWT in x-token.
	// Keep the Bearer header as well for deployments that expose the API through
	// a standards-based gateway.
	req.Header.Set("x-token", token)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode/100 != 2 {
		return nil, 0, fmt.Errorf("ThingsPanel devices %s: %s", resp.Status, string(b))
	}
	var env struct {
		Data struct {
			Total int         `json:"total"`
			List  []deviceRow `json:"list"`
		} `json:"data"`
	}
	if err = json.Unmarshal(b, &env); err != nil {
		return nil, 0, err
	}
	return env.Data.List, env.Data.Total, nil
}
func (c *Client) Health(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/health", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("ThingsPanel %s", resp.Status)
	}
	return nil
}
func jwtPayload(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid ThingsPanel JWT")
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var out map[string]any
	err = json.Unmarshal(b, &out)
	return out, err
}
func stringValue(v map[string]any, keys ...string) string {
	for _, k := range keys {
		if x, ok := v[k].(string); ok {
			return x
		}
	}
	return ""
}
func first(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
func category(kind string) string {
	if kind == "1" {
		return "gateway"
	}
	return "sensor"
}
func role(kind string) string {
	if kind == "1" {
		return "GATEWAY"
	}
	if kind == "2" || kind == "3" {
		return "CHILD"
	}
	return "DIRECT"
}
