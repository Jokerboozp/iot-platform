package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"iot-platform/internal/auth"
	"iot-platform/internal/config"
	"iot-platform/internal/core"
	"iot-platform/internal/mcpserver"
	"iot-platform/internal/metrics"
	"iot-platform/internal/model"
	"iot-platform/internal/ports"

	"github.com/gin-gonic/gin"
)

type ctxKey string

const claimsKey ctxKey = "claims"

type Server struct {
	cfg     config.Config
	engine  *core.Engine
	auth    *auth.Manager
	metrics *metrics.Registry
	log     *slog.Logger
	router  *gin.Engine
}

func New(cfg config.Config, engine *core.Engine, m *metrics.Registry, log *slog.Logger) *Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.RedirectTrailingSlash = false
	s := &Server{cfg: cfg, engine: engine, auth: auth.New(cfg.JWTSecret), metrics: m, log: log, router: router}
	router.Use(s.cors(), s.security(), s.accessLog(), s.recovery())
	s.routes()
	return s
}
func (s *Server) Handler() http.Handler { return s.router }
func (s *Server) routes() {
	s.router.POST("/api/v1/auth/login", s.endpoint(s.login))
	s.router.GET("/health/live", s.endpoint(func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]string{"status": "ok"}) }))
	s.router.GET("/health/ready", s.endpoint(s.ready))
	s.router.GET("/metrics", s.endpoint(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = io.WriteString(w, s.metrics.Prometheus())
	}))
	s.router.POST("/api/v1/integrations/video/alarm", s.endpoint(s.videoWebhook))
	s.router.GET("/api/v1/integrations/video/cameras", s.authorize("viewer"), s.endpoint(s.videoCameras))
	s.router.POST("/api/v1/integrations/video/cameras", s.authorize("operator"), s.endpoint(s.saveVideoCamera))
	s.router.PUT("/api/v1/integrations/video/cameras/:id", s.authorize("operator"), s.endpoint(s.saveVideoCamera, "id"))
	s.router.POST("/api/v1/device-ingest/:deviceId", s.endpoint(s.deviceIngest, "deviceId"))
	s.router.GET("/api/v1/products", s.authorize("viewer"), s.endpoint(s.products))
	s.router.POST("/api/v1/products", s.authorize("operator"), s.endpoint(s.saveProduct))
	s.router.PUT("/api/v1/products/:id", s.authorize("operator"), s.endpoint(s.saveProduct, "id"))
	s.router.GET("/api/v1/protocol-packages", s.authorize("viewer"), s.endpoint(s.protocolPackages))
	s.router.POST("/api/v1/protocol-packages", s.authorize("operator"), s.endpoint(s.saveProtocolPackage))
	s.router.PUT("/api/v1/protocol-packages/:id", s.authorize("operator"), s.endpoint(s.saveProtocolPackage, "id"))
	s.router.POST("/api/v1/protocol-packages/:id/test", s.authorize("operator"), s.endpoint(s.testProtocolPackage, "id"))
	s.router.GET("/api/v1/device-registry", s.authorize("viewer"), s.endpoint(s.deviceRegistry))
	s.router.POST("/api/v1/device-registry", s.authorize("operator"), s.endpoint(s.saveManagedDevice))
	s.router.PUT("/api/v1/device-registry/:id", s.authorize("operator"), s.endpoint(s.saveManagedDevice, "id"))
	s.router.POST("/api/v1/discovered-devices/:id/register", s.authorize("operator"), s.endpoint(s.registerDiscoveredDevice, "id"))
	s.router.POST("/api/v1/device-registry/:id/credentials", s.authorize("admin"), s.endpoint(s.rotateDeviceCredential, "id"))
	s.router.GET("/api/v1/device-registry/:id/connection-guide", s.authorize("viewer"), s.endpoint(s.deviceConnectionGuide, "id"))
	s.router.POST("/api/v1/device-registry/:id/debug", s.authorize("operator"), s.endpoint(s.debugDeviceIngest, "id"))
	s.router.POST("/api/v1/raw-messages", s.authorize("operator"), s.endpoint(s.ingestRaw))
	s.router.GET("/api/v1/raw-messages", s.authorize("viewer"), s.endpoint(s.listRaw))
	s.router.POST("/api/v1/raw-messages/download", s.authorize("viewer"), s.endpoint(s.downloadRawBatch))
	s.router.GET("/api/v1/raw-messages/:id", s.authorize("viewer"), s.endpoint(s.rawDetail, "id"))
	s.router.GET("/api/v1/raw-messages/:id/download", s.authorize("viewer"), s.endpoint(s.downloadRaw, "id"))
	s.router.POST("/api/v1/raw-messages/replay", s.authorize("admin"), s.endpoint(s.startReplay))
	s.router.GET("/api/v1/replays/:id", s.authorize("viewer"), s.endpoint(s.getReplay, "id"))
	s.router.GET("/api/v1/devices", s.authorize("viewer"), s.endpoint(s.devices))
	s.router.GET("/api/v1/devices/:deviceId/latest", s.authorize("viewer"), s.endpoint(s.deviceLatest, "deviceId"))
	s.router.GET("/api/v1/devices/:deviceId/properties/history", s.authorize("viewer"), s.endpoint(s.history, "deviceId"))
	s.router.POST("/api/v1/device-states", s.authorize("operator"), s.endpoint(s.stateEvent))
	s.router.GET("/api/v1/rules", s.authorize("viewer"), s.endpoint(s.rules))
	s.router.POST("/api/v1/rules", s.authorize("operator"), s.endpoint(s.saveRule))
	s.router.PUT("/api/v1/rules/:id", s.authorize("operator"), s.endpoint(s.saveRule, "id"))
	s.router.DELETE("/api/v1/rules/:id", s.authorize("operator"), s.endpoint(s.deleteRule, "id"))
	s.router.GET("/api/v1/alarms", s.authorize("viewer"), s.endpoint(s.alarms))
	s.router.GET("/api/v1/alarms/:id", s.authorize("viewer"), s.endpoint(s.alarm, "id"))
	s.router.POST("/api/v1/alarms/:id/actions", s.authorize("operator"), s.endpoint(s.alarmAction, "id"))
	s.router.GET("/api/v1/ai/alarm-analysis/:alarmId", s.authorize("viewer"), s.endpoint(s.aiAnalysis, "alarmId"))
	s.router.POST("/api/v1/ai/chat", s.authorize("viewer"), s.endpoint(s.aiChat))
	s.router.POST("/api/v1/ai/rule-draft", s.authorize("operator"), s.endpoint(s.aiRuleDraft))
	s.router.POST("/api/v1/ai/reports", s.authorize("viewer"), s.endpoint(s.aiReport))
	s.router.POST("/api/v1/knowledge/documents", s.authorize("operator"), s.endpoint(s.knowledgeUpload))
	s.router.POST("/api/v1/mqtt/token", s.authorize("viewer"), s.endpoint(s.mqttToken))
	s.router.POST("/api/v1/mqtt/load-token", s.authorize("admin"), s.endpoint(s.mqttLoadToken))
	s.router.POST("/api/v1/device-mqtt/token", s.endpoint(s.deviceMQTTToken))
	s.router.POST("/api/v1/integrations/thingspanel/sync", s.authorize("admin"), s.endpoint(s.thingsPanelSync))
	mcpHandler := gin.WrapH(mcpserver.New(s.engine))
	s.router.GET("/mcp", s.authorize("viewer"), mcpHandler)
	s.router.POST("/mcp", s.authorize("viewer"), mcpHandler)
	s.router.DELETE("/mcp", s.authorize("viewer"), mcpHandler)
	s.router.NoRoute(func(c *gin.Context) { ginProblem(c, http.StatusNotFound, "route not found") })
	s.router.NoMethod(func(c *gin.Context) { ginProblem(c, http.StatusMethodNotAllowed, "method not allowed") })
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TenantID string `json:"tenantId"`
	}
	if decode(w, r, &in) != nil {
		return
	}
	role := "admin"
	if in.Username != s.cfg.AdminUser || in.Password != s.cfg.AdminPassword {
		if s.engine.Catalog == nil {
			problem(w, 401, "invalid credentials")
			return
		}
		external, err := s.engine.Catalog.Authenticate(r.Context(), in.Username, in.Password)
		if err != nil {
			problem(w, 401, "invalid credentials")
			return
		}
		in.TenantID = external.TenantID
		role = external.Role
	}
	if in.TenantID == "" {
		in.TenantID = "tenant_001"
	}
	token, _ := s.auth.Issue(in.Username, in.TenantID, role, nil, 8*time.Hour)
	write(w, 200, map[string]any{"accessToken": token, "expiresIn": 28800, "tenantId": in.TenantID, "role": role})
}
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	checks := map[string]string{}
	status := 200
	for name, check := range map[string]func(context.Context) error{"repository": s.engine.Repo.Health, "archive": s.engine.Archive.Health, "eventBus": s.engine.Bus.Health, "realtime": s.engine.Realtime.Health} {
		if err := check(ctx); err != nil {
			checks[name] = err.Error()
			status = 503
		} else {
			checks[name] = "ok"
		}
	}
	write(w, status, map[string]any{"status": map[bool]string{true: "ok", false: "degraded"}[status == 200], "checks": checks})
}
func (s *Server) products(w http.ResponseWriter, r *http.Request) {
	items, err := s.engine.Repo.ListProducts(r.Context(), claims(r).TenantID)
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	write(w, 200, map[string]any{"items": items, "count": len(items)})
}
func (s *Server) saveProduct(w http.ResponseWriter, r *http.Request) {
	var v model.Product
	if decode(w, r, &v) != nil {
		return
	}
	c := claims(r)
	v.TenantID = c.TenantID
	if id := r.PathValue("id"); id != "" {
		v.ID = id
	}
	if v.ID == "" {
		v.ID = "product_" + randomHex(6)
	}
	if v.Name == "" || v.ProtocolPackageID == "" {
		problem(w, 422, "name and protocolPackageId are required")
		return
	}
	pkg, err := s.engine.Repo.GetProtocolPackage(r.Context(), c.TenantID, v.ProtocolPackageID)
	if err != nil {
		problem(w, 422, "protocol package not found")
		return
	}
	if v.Transport == "" {
		v.Transport = pkg.Transport
	}
	if v.PayloadFormat == "" {
		v.PayloadFormat = pkg.PayloadFormat
	}
	if v.Status == "" {
		v.Status = "ENABLED"
	}
	now := time.Now().UnixMilli()
	if old, getErr := s.engine.Repo.GetProduct(r.Context(), c.TenantID, v.ID); getErr == nil {
		v.CreatedAt = old.CreatedAt
	}
	if v.CreatedAt == 0 {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	if err = s.engine.Repo.SaveProduct(r.Context(), v); err != nil {
		problem(w, 500, err.Error())
		return
	}
	s.audit(r, "product.save", "product", v.ID, map[string]any{"status": v.Status})
	write(w, 201, v)
}
func (s *Server) protocolPackages(w http.ResponseWriter, r *http.Request) {
	items, err := s.engine.Repo.ListProtocolPackages(r.Context(), claims(r).TenantID)
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	write(w, 200, map[string]any{"items": items, "count": len(items), "parserTypes": []string{"custom_json_parser", "fire_smoke_parser", "modbus_parser"}})
}
func (s *Server) saveProtocolPackage(w http.ResponseWriter, r *http.Request) {
	var v model.ProtocolPackage
	if decode(w, r, &v) != nil {
		return
	}
	c := claims(r)
	v.TenantID = c.TenantID
	if id := r.PathValue("id"); id != "" {
		v.ID = id
	}
	if v.ID == "" {
		v.ID = "protocol_" + randomHex(6)
	}
	if v.Name == "" || v.ParserType == "" {
		problem(w, 422, "name and parserType are required")
		return
	}
	allowed := map[string]bool{"custom_json_parser": true, "fire_smoke_parser": true, "modbus_parser": true}
	if !allowed[v.ParserType] {
		problem(w, 422, "unsupported parserType")
		return
	}
	if v.Version == "" {
		v.Version = "1.0.0"
	}
	if v.Protocol == "" {
		v.Protocol = "json"
	}
	if v.Transport == "" {
		v.Transport = "MQTT"
	}
	if v.PayloadFormat == "" {
		v.PayloadFormat = "json"
	}
	if v.Status == "" {
		v.Status = "DRAFT"
	}
	if v.Status != "DRAFT" && v.Status != "PUBLISHED" && v.Status != "DISABLED" {
		problem(w, 422, "status must be DRAFT, PUBLISHED or DISABLED")
		return
	}
	now := time.Now().UnixMilli()
	if old, getErr := s.engine.Repo.GetProtocolPackage(r.Context(), c.TenantID, v.ID); getErr == nil {
		v.CreatedAt = old.CreatedAt
	}
	if v.CreatedAt == 0 {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	if err := s.engine.Repo.SaveProtocolPackage(r.Context(), v); err != nil {
		problem(w, 500, err.Error())
		return
	}
	s.audit(r, "protocol.save", "protocolPackage", v.ID, map[string]any{"version": v.Version, "status": v.Status})
	write(w, 201, v)
}
func (s *Server) testProtocolPackage(w http.ResponseWriter, r *http.Request) {
	pkg, err := s.engine.Repo.GetProtocolPackage(r.Context(), claims(r).TenantID, r.PathValue("id"))
	if err != nil {
		problem(w, 404, "protocol package not found")
		return
	}
	var in struct {
		ProductID string          `json:"productId"`
		DeviceID  string          `json:"deviceId"`
		Payload   json.RawMessage `json:"payload"`
	}
	if decode(w, r, &in) != nil {
		return
	}
	if len(in.Payload) == 0 {
		problem(w, 422, "payload is required")
		return
	}
	if in.ProductID == "" {
		in.ProductID = "protocol_test"
	}
	if in.DeviceID == "" {
		in.DeviceID = "device_test"
	}
	raw := model.RawMessage{MessageID: "raw_test_" + randomHex(6), TenantID: pkg.TenantID, ProductID: in.ProductID, DeviceID: in.DeviceID, Protocol: pkg.Protocol, Transport: pkg.Transport, PayloadFormat: pkg.PayloadFormat, Payload: in.Payload, ReceivedAt: time.Now().UnixMilli()}
	msg, err := s.engine.Parsers.ParseWith(pkg.ParserType, raw)
	if err != nil {
		write(w, 200, map[string]any{"success": false, "error": err.Error(), "raw": raw})
		return
	}
	write(w, 200, map[string]any{"success": true, "standardMessage": msg})
}
func (s *Server) deviceRegistry(w http.ResponseWriter, r *http.Request) {
	tenantID := claims(r).TenantID
	items, err := s.engine.Repo.ListManagedDevices(r.Context(), tenantID)
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	childCounts := make(map[string]int)
	for _, v := range items {
		if v.GatewayID != "" {
			childCounts[v.GatewayID]++
		}
	}
	out := make([]map[string]any, 0, len(items))
	for _, v := range items {
		row := map[string]any{"device": v, "childCount": childCounts[v.ID]}
		if state, stateErr := s.engine.Repo.GetDeviceState(r.Context(), tenantID, v.ID); stateErr == nil {
			row["runtimeState"] = state
		}
		out = append(out, row)
	}
	write(w, 200, map[string]any{"items": out, "count": len(out)})
}
func (s *Server) saveManagedDevice(w http.ResponseWriter, r *http.Request) {
	var v model.ManagedDevice
	if decode(w, r, &v) != nil {
		return
	}
	c := claims(r)
	v.TenantID = c.TenantID
	if id := r.PathValue("id"); id != "" {
		v.ID = id
	}
	if v.ID == "" {
		v.ID = "device_" + randomHex(6)
	}
	if v.Name == "" || v.ProductID == "" {
		problem(w, 422, "name and productId are required")
		return
	}
	product, err := s.engine.Repo.GetProduct(r.Context(), c.TenantID, v.ProductID)
	if err != nil {
		problem(w, 422, "product not found")
		return
	}
	now := time.Now().UnixMilli()
	credential := model.DeviceCredential{}
	if old, err := s.engine.Repo.GetManagedDevice(r.Context(), c.TenantID, v.ID); err == nil {
		v.AccessKey, v.SecretHash, v.SecretHint, v.CreatedAt = old.AccessKey, old.SecretHash, old.SecretHint, old.CreatedAt
		if v.RegistrationSource == "" {
			v.RegistrationSource = old.RegistrationSource
		}
		if old.AutoRegistered {
			v.AutoRegistered = true
		}
	} else {
		credential = newDeviceCredential()
		v.AccessKey = credential.AccessKey
		v.SecretHash = secretHash(credential.Secret)
		v.SecretHint = credential.Secret[len(credential.Secret)-6:]
		v.CreatedAt = now
	}
	if v.Status == "" {
		v.Status = "ENABLED"
	}
	if v.DeviceRole == "" {
		if product.Category == "gateway" {
			v.DeviceRole = "GATEWAY"
		} else {
			v.DeviceRole = "DIRECT"
		}
	}
	if v.DeviceRole != "DIRECT" && v.DeviceRole != "GATEWAY" && v.DeviceRole != "CHILD" {
		problem(w, 422, "deviceRole must be DIRECT, GATEWAY or CHILD")
		return
	}
	if v.RegistrationSource == "" {
		v.RegistrationSource = "MANUAL"
	}
	if v.DeviceRole == "CHILD" {
		if v.GatewayID == "" || v.GatewayID == v.ID {
			problem(w, 422, "a child device must reference a different gateway")
			return
		}
		gateway, gatewayErr := s.engine.Repo.GetManagedDevice(r.Context(), c.TenantID, v.GatewayID)
		if gatewayErr != nil {
			problem(w, 422, "gateway not found")
			return
		}
		gatewayProduct, productErr := s.engine.Repo.GetProduct(r.Context(), c.TenantID, gateway.ProductID)
		if productErr != nil || gateway.DeviceRole != "GATEWAY" && gatewayProduct.Category != "gateway" {
			problem(w, 422, "selected parent device is not a gateway")
			return
		}
	} else {
		v.GatewayID = ""
	}
	v.UpdatedAt = now
	if err := s.engine.Repo.SaveManagedDevice(r.Context(), v); err != nil {
		problem(w, 500, err.Error())
		return
	}
	s.audit(r, "device.save", "device", v.ID, map[string]any{"productId": v.ProductID, "status": v.Status})
	result := map[string]any{"device": v}
	if credential.Secret != "" {
		result["credential"] = credential
	}
	write(w, 201, result)
}
func (s *Server) registerDiscoveredDevice(w http.ResponseWriter, r *http.Request) {
	c := claims(r)
	id := r.PathValue("id")
	if _, err := s.engine.Repo.GetManagedDevice(r.Context(), c.TenantID, id); err == nil {
		problem(w, 409, "device is already registered")
		return
	}
	state, err := s.engine.Repo.GetDeviceState(r.Context(), c.TenantID, id)
	if err != nil {
		problem(w, 404, "discovered device not found")
		return
	}
	product, err := s.engine.Repo.GetProduct(r.Context(), c.TenantID, state.ProductID)
	if err != nil {
		problem(w, 422, "register its product before registering this device")
		return
	}
	credential := newDeviceCredential()
	now := time.Now().UnixMilli()
	role := "DIRECT"
	if product.Category == "gateway" {
		role = "GATEWAY"
	}
	device := model.ManagedDevice{ID: id, TenantID: c.TenantID, ProductID: state.ProductID, Name: "发现设备 " + id, Status: "ENABLED", DeviceRole: role, RegistrationSource: "DISCOVERY", AccessKey: credential.AccessKey, SecretHash: secretHash(credential.Secret), SecretHint: credential.Secret[len(credential.Secret)-6:], CreatedAt: now, UpdatedAt: now}
	if err = s.engine.Repo.SaveManagedDevice(r.Context(), device); err != nil {
		problem(w, 500, err.Error())
		return
	}
	s.audit(r, "device.discovery.register", "device", id, map[string]any{"productId": state.ProductID})
	write(w, 201, map[string]any{"device": device, "credential": credential})
}
func (s *Server) rotateDeviceCredential(w http.ResponseWriter, r *http.Request) {
	c := claims(r)
	v, err := s.engine.Repo.GetManagedDevice(r.Context(), c.TenantID, r.PathValue("id"))
	if err != nil {
		problem(w, 404, "device not found")
		return
	}
	credential := newDeviceCredential()
	v.AccessKey = credential.AccessKey
	v.SecretHash = secretHash(credential.Secret)
	v.SecretHint = credential.Secret[len(credential.Secret)-6:]
	v.UpdatedAt = time.Now().UnixMilli()
	if err = s.engine.Repo.SaveManagedDevice(r.Context(), v); err != nil {
		problem(w, 500, err.Error())
		return
	}
	s.audit(r, "device.credential.rotate", "device", v.ID, nil)
	write(w, 200, map[string]any{"deviceId": v.ID, "credential": credential})
}
func (s *Server) deviceConnectionGuide(w http.ResponseWriter, r *http.Request) {
	c := claims(r)
	v, err := s.engine.Repo.GetManagedDevice(r.Context(), c.TenantID, r.PathValue("id"))
	if err != nil {
		problem(w, 404, "device not found")
		return
	}
	p, err := s.engine.Repo.GetProduct(r.Context(), c.TenantID, v.ProductID)
	if err != nil {
		problem(w, 404, "product not found")
		return
	}
	pkg, _ := s.engine.Repo.GetProtocolPackage(r.Context(), c.TenantID, p.ProtocolPackageID)
	result := map[string]any{"deviceId": v.ID, "productId": p.ID, "deviceRole": v.DeviceRole, "gatewayId": v.GatewayID, "accessKey": v.AccessKey, "secretHint": v.SecretHint, "http": map[string]any{"method": "POST", "url": "/api/v1/device-ingest/" + v.ID, "headers": map[string]string{"X-Device-Key": v.AccessKey, "X-Device-Secret": "<仅创建或轮换时显示>", "Content-Type": "application/json"}}, "mqtt": map[string]any{"broker": "mqtt://localhost:1883", "topic": fmt.Sprintf("/external/raw/%s/%s/%s", c.TenantID, p.ID, v.ID), "tokenEndpoint": "/api/v1/device-mqtt/token", "tokenHeaders": map[string]string{"X-Device-Key": v.AccessKey, "X-Device-Secret": "<仅创建或轮换时显示>"}}, "payloadTemplate": map[string]any{"messageId": "raw_<unique>", "tenantId": c.TenantID, "productId": p.ID, "deviceId": v.ID, "protocol": pkg.Protocol, "transport": pkg.Transport, "payloadFormat": pkg.PayloadFormat, "payload": map[string]any{"properties": map[string]any{"temperature": 25.5}}}}
	if v.DeviceRole == "GATEWAY" || p.Category == "gateway" {
		result["gateway"] = map[string]any{"autoRegisterChildren": true, "description": "网关上报一个尚未注册的子设备时，平台自动注册并建立关联", "childPayloadTemplate": map[string]any{"messageId": "raw_<unique>", "deviceId": "child_device_001", "deviceName": "一号子设备", "productId": "<child_product_id>", "payload": map[string]any{"properties": map[string]any{"temperature": 25.5}}}}
	}
	write(w, 200, result)
}
func (s *Server) debugDeviceIngest(w http.ResponseWriter, r *http.Request) {
	c := claims(r)
	v, err := s.engine.Repo.GetManagedDevice(r.Context(), c.TenantID, r.PathValue("id"))
	if err != nil {
		problem(w, 404, "device not found")
		return
	}
	var raw model.RawMessage
	if decode(w, r, &raw) != nil {
		return
	}
	if err = s.prepareManagedRaw(r.Context(), &raw, v); err != nil {
		problem(w, 422, err.Error())
		return
	}
	idx, created, err := s.engine.IngestRaw(r.Context(), raw)
	if err != nil {
		problem(w, 422, err.Error())
		return
	}
	s.audit(r, "device.debug.ingest", "device", v.ID, map[string]any{"messageId": idx.MessageID})
	write(w, map[bool]int{true: 201, false: 200}[created], map[string]any{"created": created, "archive": idx})
}
func (s *Server) deviceIngest(w http.ResponseWriter, r *http.Request) {
	accessKey, secret := r.Header.Get("X-Device-Key"), r.Header.Get("X-Device-Secret")
	v, err := s.engine.Repo.GetManagedDeviceByAccessKey(r.Context(), accessKey)
	if err != nil || v.ID != r.PathValue("deviceId") || v.Status != "ENABLED" || !hmac.Equal([]byte(v.SecretHash), []byte(secretHash(secret))) {
		problem(w, 401, "invalid or disabled device credential")
		return
	}
	if tenantID := r.Header.Get("X-Tenant-ID"); tenantID != "" && tenantID != v.TenantID {
		problem(w, 401, "invalid or disabled device credential")
		return
	}
	var raw model.RawMessage
	if decode(w, r, &raw) != nil {
		return
	}
	if err = s.prepareManagedRaw(r.Context(), &raw, v); err != nil {
		problem(w, 422, err.Error())
		return
	}
	idx, created, err := s.engine.IngestRaw(r.Context(), raw)
	if err != nil {
		problem(w, 422, err.Error())
		return
	}
	write(w, map[bool]int{true: 201, false: 200}[created], map[string]any{"created": created, "messageId": idx.MessageID, "receivedAt": idx.ReceivedAt})
}
func (s *Server) prepareManagedRaw(ctx context.Context, raw *model.RawMessage, device model.ManagedDevice) error {
	targetProductID := device.ProductID
	if raw.DeviceID != "" && raw.DeviceID != device.ID {
		if raw.ProductID == "" {
			return fmt.Errorf("child productId is required")
		}
		raw.GatewayID = device.ID
		targetProductID = raw.ProductID
		raw.Source = "gateway"
	} else {
		raw.DeviceID = device.ID
		raw.GatewayID = ""
		raw.Source = "managed-device"
	}
	product, err := s.engine.Repo.GetProduct(ctx, device.TenantID, targetProductID)
	if err != nil || product.Status != "ENABLED" {
		return fmt.Errorf("product is not enabled")
	}
	pkg, err := s.engine.Repo.GetProtocolPackage(ctx, device.TenantID, product.ProtocolPackageID)
	if err != nil || pkg.Status != "PUBLISHED" {
		return fmt.Errorf("protocol package is not published")
	}
	raw.TenantID, raw.ProductID = device.TenantID, product.ID
	raw.Protocol, raw.Transport, raw.PayloadFormat = pkg.Protocol, pkg.Transport, pkg.PayloadFormat
	return nil
}
func (s *Server) ingestRaw(w http.ResponseWriter, r *http.Request) {
	var v model.RawMessage
	if decode(w, r, &v) != nil {
		return
	}
	c := claims(r)
	v.TenantID = tenant(c, v.TenantID)
	start := time.Now()
	idx, created, err := s.engine.IngestRaw(r.Context(), v)
	s.metrics.ObserveMS("raw_archive_latency_ms", start)
	s.metrics.ObserveMS("storage_latency_ms", start)
	if err != nil {
		s.metrics.Inc("raw_archive_failed_total")
		problem(w, 422, err.Error())
		return
	}
	write(w, map[bool]int{true: 201, false: 200}[created], map[string]any{"created": created, "archive": idx})
}
func (s *Server) listRaw(w http.ResponseWriter, r *http.Request) {
	c := claims(r)
	q := r.URL.Query()
	items, err := s.engine.Repo.ListRawIndexes(r.Context(), ports.RawFilter{TenantID: c.TenantID, ProductID: q.Get("productId"), DeviceID: q.Get("deviceId"), Start: i64(q.Get("start")), End: i64(q.Get("end")), Limit: intval(q.Get("limit"), 100), Offset: intval(q.Get("offset"), 0)})
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	write(w, 200, map[string]any{"items": items, "count": len(items)})
}
func (s *Server) rawDetail(w http.ResponseWriter, r *http.Request) {
	idx, err := s.engine.Repo.GetRawIndex(r.Context(), claims(r).TenantID, r.PathValue("id"))
	if err != nil {
		problem(w, 404, "raw message not found")
		return
	}
	raw, err := s.engine.Archive.GetRaw(r.Context(), idx)
	if err != nil {
		problem(w, 500, "raw archive could not be read")
		return
	}
	write(w, 200, map[string]any{"archive": idx, "message": raw})
}
func (s *Server) downloadRaw(w http.ResponseWriter, r *http.Request) {
	idx, err := s.engine.Repo.GetRawIndex(r.Context(), claims(r).TenantID, r.PathValue("id"))
	if err != nil {
		problem(w, 404, "raw message not found")
		return
	}
	raw, err := s.engine.Archive.GetRaw(r.Context(), idx)
	if err != nil {
		problem(w, 500, "raw archive could not be read")
		return
	}
	body, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	filename := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, idx.MessageID) + ".json"
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("X-Content-SHA256", idx.PayloadHash)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append(body, '\n'))
	s.audit(r, "raw.download", "raw-message", idx.MessageID, map[string]any{"payloadHash": idx.PayloadHash})
}
func (s *Server) downloadRawBatch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MessageIDs []string `json:"messageIds"`
	}
	if decode(w, r, &in) != nil {
		return
	}
	if len(in.MessageIDs) == 0 {
		problem(w, 400, "at least one messageId is required")
		return
	}
	if len(in.MessageIDs) > 500 {
		problem(w, 422, "no more than 500 raw messages may be downloaded at once")
		return
	}
	type archivedRaw struct {
		Index   model.RawArchiveIndex
		Message model.RawMessage
	}
	items := make([]archivedRaw, 0, len(in.MessageIDs))
	seen := make(map[string]struct{}, len(in.MessageIDs))
	totalPayloadSize := 0
	for _, id := range in.MessageIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		idx, err := s.engine.Repo.GetRawIndex(r.Context(), claims(r).TenantID, id)
		if err != nil {
			problem(w, 404, "raw message not found: "+id)
			return
		}
		totalPayloadSize += idx.PayloadSize
		if totalPayloadSize > 100*1024*1024 {
			problem(w, 413, "selected raw messages exceed the 100 MiB batch limit")
			return
		}
		raw, err := s.engine.Archive.GetRaw(r.Context(), idx)
		if err != nil {
			problem(w, 500, "raw archive could not be read: "+id)
			return
		}
		items = append(items, archivedRaw{Index: idx, Message: raw})
	}
	if len(items) == 0 {
		problem(w, 400, "at least one valid messageId is required")
		return
	}
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	manifest := make([]model.RawArchiveIndex, 0, len(items))
	for i, item := range items {
		body, err := json.MarshalIndent(item.Message, "", "  ")
		if err != nil {
			problem(w, 500, err.Error())
			return
		}
		name := fmt.Sprintf("报文/%03d_%s.json", i+1, safeAttachmentName(item.Index.MessageID))
		file, err := zw.Create(name)
		if err != nil {
			problem(w, 500, err.Error())
			return
		}
		if _, err = file.Write(append(body, '\n')); err != nil {
			problem(w, 500, err.Error())
			return
		}
		manifest = append(manifest, item.Index)
	}
	manifestBody, _ := json.MarshalIndent(map[string]any{"exportedAt": time.Now().UnixMilli(), "count": len(items), "items": manifest}, "", "  ")
	manifestFile, err := zw.Create("清单.json")
	if err == nil {
		_, err = manifestFile.Write(append(manifestBody, '\n'))
	}
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	if err = zw.Close(); err != nil {
		problem(w, 500, err.Error())
		return
	}
	filename := fmt.Sprintf("原始报文_%s_%d条.zip", time.Now().Format("20060102_150405"), len(items))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="raw-messages.zip"; filename*=UTF-8''%s`, url.QueryEscape(filename)))
	w.Header().Set("X-Archive-Count", strconv.Itoa(len(items)))
	w.Header().Set("Content-Length", strconv.Itoa(archive.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = archive.WriteTo(w)
	s.audit(r, "raw.download.batch", "raw-message", "batch", map[string]any{"count": len(items), "payloadBytes": totalPayloadSize})
}

func safeAttachmentName(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, value)
}
func (s *Server) startReplay(w http.ResponseWriter, r *http.Request) {
	var v model.ReplayRequest
	if decode(w, r, &v) != nil {
		return
	}
	c := claims(r)
	v.TenantID = c.TenantID
	v.CreatedBy = c.Username
	task, err := s.engine.StartReplay(r.Context(), v)
	if err != nil {
		problem(w, 422, err.Error())
		return
	}
	write(w, 202, task)
}
func (s *Server) getReplay(w http.ResponseWriter, r *http.Request) {
	v, err := s.engine.Repo.GetReplay(r.Context(), r.PathValue("id"))
	if err != nil {
		problem(w, 404, "replay not found")
		return
	}
	if v.TenantID != claims(r).TenantID {
		problem(w, 404, "replay not found")
		return
	}
	write(w, 200, v)
}
func (s *Server) devices(w http.ResponseWriter, r *http.Request) {
	items, err := s.engine.Repo.ListDeviceStates(r.Context(), claims(r).TenantID)
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	online := 0
	for _, v := range items {
		if v.BusinessStatus == "ONLINE" {
			online++
		}
	}
	write(w, 200, map[string]any{"items": items, "total": len(items), "online": online, "offline": len(items) - online})
}
func (s *Server) deviceLatest(w http.ResponseWriter, r *http.Request) {
	tenant, device := claims(r).TenantID, r.PathValue("deviceId")
	v, err := s.engine.Repo.GetDeviceState(r.Context(), tenant, device)
	if err != nil {
		problem(w, 404, "device state not found")
		return
	}
	out := map[string]any{"state": v}
	if latest, latestErr := s.engine.Repo.GetLatestMessage(r.Context(), tenant, device); latestErr == nil {
		out["latestMessage"] = latest
		out["properties"] = latest.Properties
		out["timestamp"] = latest.Timestamp
	}
	write(w, 200, out)
}
func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	property := q.Get("property")
	if property == "" {
		problem(w, 400, "property is required")
		return
	}
	items, err := s.engine.Repo.PropertyHistory(r.Context(), claims(r).TenantID, r.PathValue("deviceId"), property, i64(q.Get("start")), i64(q.Get("end")), intval(q.Get("limit"), 1000))
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	write(w, 200, map[string]any{"items": items})
}
func (s *Server) stateEvent(w http.ResponseWriter, r *http.Request) {
	var v model.DeviceState
	if decode(w, r, &v) != nil {
		return
	}
	v.TenantID = tenant(claims(r), v.TenantID)
	if err := s.engine.UpdateDeviceState(r.Context(), v); err != nil {
		problem(w, 422, err.Error())
		return
	}
	write(w, 202, v)
}
func (s *Server) rules(w http.ResponseWriter, r *http.Request) {
	v, err := s.engine.Repo.ListRules(r.Context(), claims(r).TenantID)
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	write(w, 200, map[string]any{"items": v})
}
func (s *Server) saveRule(w http.ResponseWriter, r *http.Request) {
	var v model.AlarmRule
	if decode(w, r, &v) != nil {
		return
	}
	c := claims(r)
	v.TenantID = c.TenantID
	status := http.StatusCreated
	if id := r.PathValue("id"); id != "" {
		status = http.StatusOK
		v.ID = id
		items, err := s.engine.Repo.ListRules(r.Context(), c.TenantID)
		if err != nil {
			problem(w, 500, err.Error())
			return
		}
		found := false
		for _, current := range items {
			if current.ID == id {
				v.CreatedAt = current.CreatedAt
				v.Version = current.Version + 1
				found = true
				break
			}
		}
		if !found {
			problem(w, 404, "rule not found")
			return
		}
	} else if v.ID == "" {
		v.ID = fmt.Sprintf("rule_%d", time.Now().UnixNano())
	}
	if v.Version == 0 {
		v.Version = 1
	}
	now := time.Now().UnixMilli()
	if v.CreatedAt == 0 {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	if len(v.Conditions) == 0 && strings.TrimSpace(v.Expression) == "" {
		problem(w, 422, "at least one condition or a Gengine expression is required")
		return
	}
	if v.Expression != "" {
		if err := core.ValidateGengineExpression(v.Expression); err != nil {
			problem(w, 422, err.Error())
			return
		}
	}
	_, conflicts, validationErr := s.engine.ValidateRuleDraft(r.Context(), v)
	if validationErr != nil {
		problem(w, 422, validationErr.Error())
		return
	}
	if len(conflicts) > 0 && !strings.EqualFold(r.URL.Query().Get("confirmConflicts"), "true") {
		write(w, 409, map[string]any{"type": "rule-conflict", "detail": "rule conflicts require explicit confirmation", "conflicts": conflicts})
		return
	}
	if err := s.engine.Repo.SaveRule(r.Context(), v); err != nil {
		problem(w, 500, err.Error())
		return
	}
	s.audit(r, "rule.save", "rule", v.ID, map[string]any{"version": v.Version, "enabled": v.Enabled})
	write(w, status, v)
}
func (s *Server) deleteRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.engine.Repo.DeleteRule(r.Context(), claims(r).TenantID, id); err != nil {
		problem(w, 404, "rule not found")
		return
	}
	s.audit(r, "rule.delete", "rule", id, nil)
	write(w, 200, map[string]any{"deleted": true, "id": id})
}
func (s *Server) alarms(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, err := s.engine.Repo.ListAlarms(r.Context(), ports.AlarmFilter{TenantID: claims(r).TenantID, DeviceID: q.Get("deviceId"), Status: q.Get("status"), Level: q.Get("level"), Source: q.Get("source"), Start: i64(q.Get("start")), End: i64(q.Get("end")), Limit: intval(q.Get("limit"), 100), Offset: intval(q.Get("offset"), 0)})
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	write(w, 200, map[string]any{"items": items, "count": len(items)})
}
func (s *Server) alarm(w http.ResponseWriter, r *http.Request) {
	v, err := s.engine.Repo.GetAlarm(r.Context(), claims(r).TenantID, r.PathValue("id"))
	if err != nil {
		problem(w, 404, "alarm not found")
		return
	}
	write(w, 200, v)
}
func (s *Server) alarmAction(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action string `json:"action"`
	}
	if decode(w, r, &in) != nil {
		return
	}
	v, err := s.engine.SetAlarmStatus(r.Context(), claims(r).TenantID, r.PathValue("id"), strings.ToUpper(in.Action), claims(r).Username)
	if err != nil {
		problem(w, 422, err.Error())
		return
	}
	write(w, 200, v)
}
func (s *Server) aiAnalysis(w http.ResponseWriter, r *http.Request) {
	v, err := s.engine.Repo.GetAIAnalysis(r.Context(), r.PathValue("alarmId"))
	if err != nil {
		problem(w, 404, "analysis not found or still pending")
		return
	}
	write(w, 200, v)
}
func (s *Server) aiChat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Question string `json:"question"`
	}
	if decode(w, r, &in) != nil {
		return
	}
	answer, err := s.engine.OpsChat(r.Context(), claims(r).TenantID, in.Question)
	if err != nil {
		problem(w, 502, err.Error())
		return
	}
	write(w, 200, map[string]string{"answer": answer})
}
func (s *Server) aiRuleDraft(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	if decode(w, r, &in) != nil {
		return
	}
	rule, err := s.engine.AI.RuleDraft(r.Context(), claims(r).TenantID, in.Text)
	if err != nil {
		problem(w, 502, err.Error())
		return
	}
	c := claims(r)
	rule.TenantID = c.TenantID
	rule.Enabled = false
	if rule.ID == "" {
		rule.ID = "rule_draft_" + randomHex(8)
	}
	if rule.Match == "" {
		rule.Match = "all"
	}
	if rule.Version == 0 {
		rule.Version = 1
	}
	if rule.Level == "" {
		rule.Level = "MEDIUM"
	}
	warnings, conflicts, validationErr := s.engine.ValidateRuleDraft(r.Context(), rule)
	if validationErr != nil {
		_ = s.engine.Repo.SaveAIToolCall(r.Context(), model.AIToolCallLog{ID: "tool_" + randomHex(8), TenantID: c.TenantID, Actor: c.Username, Tool: "ai.rule_draft.validate", Input: map[string]any{"text": in.Text}, Output: rule, Success: false, Error: validationErr.Error(), CreatedAt: time.Now().UnixMilli()})
		problem(w, 422, validationErr.Error())
		return
	}
	_ = s.engine.Repo.SaveAudit(r.Context(), model.AuditLog{ID: fmt.Sprintf("audit_%d", time.Now().UnixNano()), TenantID: c.TenantID, Actor: c.Username, Action: "ai.rule_draft", TargetType: "rule", TargetID: rule.ID, Details: map[string]any{"success": true}, CreatedAt: time.Now().UnixMilli()})
	_ = s.engine.Repo.SaveAIToolCall(r.Context(), model.AIToolCallLog{ID: "tool_" + randomHex(8), TenantID: c.TenantID, Actor: c.Username, Tool: "ai.rule_draft", Input: map[string]any{"text": in.Text}, Output: rule, Success: true, CreatedAt: time.Now().UnixMilli()})
	write(w, 200, map[string]any{"draft": rule, "requiresHumanApproval": true, "schemaValid": true, "warnings": warnings, "conflicts": conflicts})
}

func (s *Server) aiReport(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Period string `json:"period"`
		Start  int64  `json:"start"`
		End    int64  `json:"end"`
	}
	if decode(w, r, &in) != nil {
		return
	}
	if in.Period == "" {
		in.Period = "日报"
	}
	report, err := s.engine.GenerateReport(r.Context(), claims(r).TenantID, in.Period, in.Start, in.End)
	if err != nil {
		problem(w, 502, err.Error())
		return
	}
	write(w, 200, map[string]any{"period": in.Period, "start": in.Start, "end": in.End, "report": report})
}
func (s *Server) knowledgeUpload(w http.ResponseWriter, r *http.Request) {
	if s.engine.KB == nil {
		problem(w, 503, "knowledge base disabled")
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		problem(w, 400, "invalid multipart form")
		return
	}
	f, h, err := r.FormFile("file")
	if err != nil {
		problem(w, 400, "file is required")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 32<<20))
	if err != nil {
		problem(w, 400, err.Error())
		return
	}
	id := fmt.Sprintf("doc_%d", time.Now().UnixNano())
	c := claims(r)
	productID := r.FormValue("productId")
	filename := strings.NewReplacer("/", "_", "\\", "_", "..", "_").Replace(h.Filename)
	bucket := "iot-knowledge-docs"
	objectKey := fmt.Sprintf("%s/%s/%s/%s", c.TenantID, productID, id, filename)
	if _, err = s.engine.Archive.PutObject(r.Context(), bucket, objectKey, bytes.NewReader(data), int64(len(data)), h.Header.Get("Content-Type")); err != nil {
		problem(w, 502, "store document: "+err.Error())
		return
	}
	textContent, extractErr := core.ExtractKnowledgeText(h.Filename, data)
	if extractErr != nil {
		problem(w, 422, extractErr.Error())
		return
	}
	chunks := core.ChunkKnowledgeText(textContent, 1200, 200)
	if len(chunks) == 0 {
		problem(w, 422, "document contains no indexable text")
		return
	}
	for i, chunk := range chunks {
		chunkID := fmt.Sprintf("%s-chunk-%04d", id, i+1)
		if err = s.engine.KB.Index(r.Context(), c.TenantID, productID, chunkID, []byte(chunk)); err != nil {
			problem(w, 502, err.Error())
			return
		}
	}
	doc := model.KnowledgeDoc{ID: id, TenantID: c.TenantID, ProductID: productID, ObjectBucket: bucket, ObjectKey: objectKey, Filename: h.Filename, Status: "INDEXED", Metadata: map[string]any{"size": len(data), "contentType": h.Header.Get("Content-Type"), "chunks": len(chunks), "characters": len([]rune(textContent))}, CreatedAt: time.Now().UnixMilli()}
	if err = s.engine.Repo.SaveKnowledgeDoc(r.Context(), doc); err != nil {
		problem(w, 500, err.Error())
		return
	}
	write(w, 201, doc)
}
func (s *Server) mqttToken(w http.ResponseWriter, r *http.Request) {
	c := claims(r)
	scope := []string{fmt.Sprintf("/iot/alarm/%s/#", c.TenantID), fmt.Sprintf("/iot/device/state/%s/#", c.TenantID)}
	acl := make([]auth.ACLRule, 0, len(scope))
	for _, topic := range scope {
		acl = append(acl, auth.ACLRule{Permission: "allow", Action: "subscribe", Topic: topic})
	}
	token, err := s.auth.IssueWithACL(c.Username, c.TenantID, c.Role, scope, acl, 15*time.Minute)
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	write(w, 200, map[string]any{"username": c.Username, "token": token, "expiresIn": 900, "subscriptions": scope, "websocketUrl": s.mqttWebSocketURL(r)})
}

func (s *Server) deviceMQTTToken(w http.ResponseWriter, r *http.Request) {
	accessKey, secret := r.Header.Get("X-Device-Key"), r.Header.Get("X-Device-Secret")
	v, err := s.engine.Repo.GetManagedDeviceByAccessKey(r.Context(), accessKey)
	if err != nil || v.Status != "ENABLED" || !hmac.Equal([]byte(v.SecretHash), []byte(secretHash(secret))) {
		problem(w, 401, "invalid device credentials")
		return
	}
	topic := fmt.Sprintf("/external/raw/%s/%s/%s", v.TenantID, v.ProductID, v.ID)
	acl := []auth.ACLRule{{Permission: "allow", Action: "publish", Topic: topic}, {Permission: "allow", Action: "subscribe", Topic: fmt.Sprintf("/iot/device/command/%s/%s", v.TenantID, v.ID)}}
	token, err := s.auth.IssueWithACL(v.AccessKey, v.TenantID, "device", nil, acl, 24*time.Hour)
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	write(w, 200, map[string]any{"username": v.AccessKey, "token": token, "expiresIn": 86400, "publishTopic": topic, "websocketUrl": s.mqttWebSocketURL(r)})
}

func (s *Server) mqttLoadToken(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ProductID string `json:"productId"`
	}
	if decode(w, r, &input) != nil {
		return
	}
	if input.ProductID == "" {
		problem(w, 422, "productId is required")
		return
	}
	c := claims(r)
	topic := fmt.Sprintf("/external/raw/%s/%s/#", c.TenantID, input.ProductID)
	acl := []auth.ACLRule{{Permission: "allow", Action: "publish", Topic: topic}}
	token, err := s.auth.IssueWithACL("loadgen:"+c.Username, c.TenantID, "loadgen", nil, acl, time.Hour)
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	s.audit(r, "mqtt.load-token.issue", "product", input.ProductID, map[string]any{"topic": topic, "expiresIn": 3600})
	write(w, 200, map[string]any{"username": "loadgen:" + c.Username, "token": token, "expiresIn": 3600, "publishTopicPrefix": strings.TrimSuffix(topic, "#")})
}

func (s *Server) mqttWebSocketURL(r *http.Request) string {
	if s.cfg.MQTTWebSocketURL != "" {
		return s.cfg.MQTTWebSocketURL
	}
	scheme := "ws"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "wss"
	}
	host := strings.Split(r.Host, ":")[0]
	return fmt.Sprintf("%s://%s:8083/mqtt", scheme, host)
}
func (s *Server) thingsPanelSync(w http.ResponseWriter, r *http.Request) {
	if s.engine.Catalog == nil {
		problem(w, 503, "ThingsPanel integration is disabled")
		return
	}
	result, err := s.engine.Catalog.Sync(r.Context(), claims(r).TenantID)
	if err != nil {
		problem(w, 502, err.Error())
		return
	}
	s.audit(r, "thingspanel.sync", "catalog", claims(r).TenantID, map[string]any{"devices": result.Devices, "products": result.Products})
	write(w, 200, result)
}
func (s *Server) videoWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		problem(w, 400, err.Error())
		return
	}
	platform := r.Header.Get("X-Video-Platform-ID")
	timestamp := r.Header.Get("X-Timestamp")
	secret := s.cfg.VideoSecrets[platform]
	if secret == "" && !s.cfg.DevMode {
		problem(w, 401, "unknown video platform")
		return
	}
	if secret != "" && !verifySignature(secret, timestamp, body, r.Header.Get("X-Signature")) {
		problem(w, 401, "invalid signature")
		return
	}
	ts, _ := strconv.ParseInt(timestamp, 10, 64)
	if secret != "" && abs(time.Now().Unix()-ts) > 300 {
		problem(w, 401, "stale timestamp")
		return
	}
	var v model.VideoAlarmEvent
	if err = json.Unmarshal(body, &v); err != nil {
		problem(w, 400, "invalid JSON")
		return
	}
	a, created, err := s.engine.IngestVideo(r.Context(), v)
	if err != nil {
		problem(w, 422, err.Error())
		return
	}
	write(w, map[bool]int{true: 201, false: 200}[created], map[string]any{"created": created, "alarm": a})
}
func (s *Server) videoCameras(w http.ResponseWriter, r *http.Request) {
	items, err := s.engine.Repo.ListVideoCameraMappings(r.Context(), claims(r).TenantID)
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	write(w, 200, map[string]any{"items": items, "count": len(items)})
}
func (s *Server) saveVideoCamera(w http.ResponseWriter, r *http.Request) {
	var v model.VideoCameraMapping
	if decode(w, r, &v) != nil {
		return
	}
	c := claims(r)
	v.TenantID = c.TenantID
	if id := r.PathValue("id"); id != "" {
		v.CameraID = id
	}
	if v.CameraID == "" || v.CameraName == "" || v.AreaID == "" {
		problem(w, 422, "cameraId, cameraName and areaId are required")
		return
	}
	v.UpdatedAt = time.Now().UnixMilli()
	if err := s.engine.Repo.SaveVideoCameraMapping(r.Context(), v); err != nil {
		problem(w, 500, err.Error())
		return
	}
	s.audit(r, "video.camera.save", "video-camera", v.CameraID, map[string]any{"areaId": v.AreaID, "enabled": v.Enabled})
	write(w, map[bool]int{true: 200, false: 201}[r.Method == http.MethodPut], v)
}
func randomHex(size int) string {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
func newDeviceCredential() model.DeviceCredential {
	return model.DeviceCredential{AccessKey: "dk_" + randomHex(8), Secret: "ds_" + randomHex(18)}
}
func secretHash(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}
func (s *Server) audit(r *http.Request, action, targetType, targetID string, details map[string]any) {
	c := claims(r)
	_ = s.engine.Repo.SaveAudit(r.Context(), model.AuditLog{ID: "audit_" + randomHex(10), TenantID: c.TenantID, Actor: c.Username, Action: action, TargetType: targetType, TargetID: targetID, Details: details, CreatedAt: time.Now().UnixMilli()})
}
func verifySignature(secret, ts string, body []byte, sig string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(ts))
	_, _ = mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(strings.ToLower(sig)), []byte(expected))
}

type endpointHandler func(http.ResponseWriter, *http.Request)

// endpoint adapts the established net/http business handlers to Gin while
// preserving Request.PathValue for code that reads named route parameters.
func (s *Server) endpoint(handler endpointHandler, pathParams ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, name := range pathParams {
			c.Request.SetPathValue(name, c.Param(name))
		}
		handler(c.Writer, c.Request)
	}
}

func (s *Server) authorize(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := auth.Bearer(c.GetHeader("Authorization"))
		claimsValue, err := s.auth.Parse(token)
		if err != nil {
			ginProblem(c, http.StatusUnauthorized, err.Error())
			c.Abort()
			return
		}
		allowed := claimsValue.Role == "admin" || claimsValue.Role == role || role == "viewer" && (claimsValue.Role == "operator" || claimsValue.Role == "viewer")
		if !allowed {
			ginProblem(c, http.StatusForbidden, "insufficient role")
			c.Abort()
			return
		}
		ctx := auth.ContextWithClaims(context.WithValue(c.Request.Context(), claimsKey, claimsValue), claimsValue)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func (s *Server) security() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Content-Security-Policy", "default-src 'self'; connect-src 'self' ws: wss:; style-src 'self' 'unsafe-inline'; script-src 'self'; worker-src 'self' blob:")
		c.Next()
	}
}

func (s *Server) cors() gin.HandlerFunc {
	allowedOrigins := make(map[string]struct{}, len(s.cfg.CORSAllowedOrigins))
	for _, origin := range s.cfg.CORSAllowedOrigins {
		allowedOrigins[strings.TrimRight(origin, "/")] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := strings.TrimRight(c.GetHeader("Origin"), "/")
		if origin != "" {
			if _, allowed := allowedOrigins[origin]; allowed {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Device-Key, X-Device-Secret, X-Video-Platform-ID, X-Timestamp, X-Signature")
				c.Header("Access-Control-Max-Age", "600")
				c.Header("Vary", "Origin")
			} else if c.Request.Method == http.MethodOptions {
				ginProblem(c, http.StatusForbidden, "origin is not allowed")
				c.Abort()
				return
			}
		}
		if c.Request.Method == http.MethodOptions {
			c.Status(http.StatusNoContent)
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *Server) accessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		if s.log != nil {
			s.log.Info("http request", "method", c.Request.Method, "path", c.Request.URL.Path, "route", c.FullPath(), "status", c.Writer.Status(), "duration", time.Since(start).String())
		}
	}
}

func (s *Server) recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		if s.log != nil {
			s.log.Error("http panic recovered", "method", c.Request.Method, "path", c.Request.URL.Path, "error", fmt.Sprint(recovered))
		}
		ginProblem(c, http.StatusInternalServerError, "internal server error")
		c.Abort()
	})
}

func ginProblem(c *gin.Context, status int, detail string) {
	c.JSON(status, gin.H{"type": "about:blank", "title": http.StatusText(status), "status": status, "detail": detail})
}
func claims(r *http.Request) auth.Claims {
	v, _ := r.Context().Value(claimsKey).(auth.Claims)
	return v
}
func tenant(c auth.Claims, requested string) string {
	if requested == "" || requested == c.TenantID {
		return c.TenantID
	}
	return c.TenantID
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		problem(w, 400, "invalid request: "+err.Error())
		return err
	}
	return nil
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, detail string) {
	write(w, status, map[string]any{"type": "about:blank", "title": http.StatusText(status), "status": status, "detail": detail})
}
func i64(v string) int64 { n, _ := strconv.ParseInt(v, 10, 64); return n }
func intval(v string, d int) int {
	n, e := strconv.Atoi(v)
	if e != nil {
		return d
	}
	return n
}
func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

var _ multipart.File
