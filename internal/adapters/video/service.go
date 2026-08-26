package video

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"iot-platform/internal/model"
)

const (
	DirectMode     = "direct"
	DahuaSDKMode   = "dahua_sdk"
	HikvisionMode  = "hikvision_sdk"
	maxSDKResponse = 1 << 20
)

type Config struct {
	ZLMAPIURL          string
	ZLMPlaybackBaseURL string
	ZLMSecret          string
	ZLMVhost           string
	ZLMApp             string
	DahuaSDKURL        string
	DahuaSDKToken      string
	HikvisionAPIURL    string
	HikvisionResolver  StreamResolver
	AllowedSourceHosts []string
}

// StreamRequest is the provider-neutral input used by an in-process vendor
// adapter. Credentials are represented by a reference only; the platform
// never receives or persists vendor passwords.
type StreamRequest struct {
	Provider      string
	TenantID      string
	CameraID      string
	CredentialRef string
	Endpoint      string
}

// StreamResolver obtains a short-lived vendor stream URL. A vendor adapter is
// injected so preview and ZLMediaKit stay independent from one SDK package.
type StreamResolver interface {
	Resolve(context.Context, StreamRequest) (model.VideoStream, error)
}

type Service struct {
	config  Config
	client  *http.Client
	gateway *zlmGateway
}

func New(config Config) (*Service, error) {
	config.ZLMAPIURL = strings.TrimRight(strings.TrimSpace(config.ZLMAPIURL), "/")
	config.ZLMPlaybackBaseURL = strings.TrimRight(strings.TrimSpace(config.ZLMPlaybackBaseURL), "/")
	config.ZLMVhost = strings.TrimSpace(config.ZLMVhost)
	if config.ZLMVhost == "" {
		config.ZLMVhost = "__defaultVhost__"
	}
	config.ZLMApp = strings.Trim(strings.TrimSpace(config.ZLMApp), "/")
	if config.ZLMApp == "" {
		config.ZLMApp = "iot"
	}
	if config.ZLMAPIURL == "" && config.DahuaSDKURL == "" && config.HikvisionAPIURL == "" && config.HikvisionResolver == nil {
		return nil, nil
	}
	client := &http.Client{Timeout: 20 * time.Second}
	service := &Service{config: config, client: client}
	if config.ZLMAPIURL != "" {
		if config.ZLMPlaybackBaseURL == "" {
			return nil, errors.New("ZLMediaKit playback base URL is required when its API URL is configured")
		}
		if _, err := absoluteHTTPURL(config.ZLMAPIURL); err != nil {
			return nil, fmt.Errorf("invalid ZLMediaKit API URL: %w", err)
		}
		if _, err := absoluteHTTPURL(config.ZLMPlaybackBaseURL); err != nil {
			return nil, fmt.Errorf("invalid ZLMediaKit playback base URL: %w", err)
		}
		service.gateway = &zlmGateway{
			apiURL:       config.ZLMAPIURL,
			playbackBase: config.ZLMPlaybackBaseURL,
			secret:       config.ZLMSecret,
			vhost:        config.ZLMVhost,
			app:          config.ZLMApp,
			client:       client,
			proxies:      make(map[string]zlmProxy),
		}
	}
	return service, nil
}

func (s *Service) Preview(ctx context.Context, camera model.VideoCameraMapping) (model.VideoPreview, error) {
	source, err := s.resolveSource(ctx, camera)
	if err != nil {
		return model.VideoPreview{}, err
	}
	if s.gateway != nil && gatewayStreamType(source) {
		if !s.sourceHostAllowed(source.URL) {
			return model.VideoPreview{}, errors.New("camera source host is not allowlisted for ZLMediaKit ingest")
		}
		preview, previewErr := s.gateway.proxy(ctx, camera.TenantID, camera.CameraID, source)
		if previewErr == nil {
			preview.CameraName = camera.CameraName
		}
		return preview, previewErr
	}
	streamType := sourceType(source.URL, source.StreamType)
	if streamType != "hls" && streamType != "mp4" && streamType != "webm" && streamType != "native" {
		return model.VideoPreview{}, errors.New("browser preview requires ZLMediaKit for RTSP/RTMP or a browser-compatible stream")
	}
	return model.VideoPreview{CameraID: camera.CameraID, CameraName: camera.CameraName, PlaybackURL: source.URL, StreamType: streamType, Provider: source.Provider, ExpiresAt: source.ExpiresAt}, nil
}

func (s *Service) Eligible(camera model.VideoCameraMapping, allowedOrigins []string) bool {
	if !camera.Enabled {
		return false
	}
	mode := normalizeMode(camera.IngestMode)
	if mode != DirectMode {
		if mode == HikvisionMode && s.config.HikvisionResolver == nil {
			return false
		}
		endpoint := s.sdkEndpoint(mode, camera)
		if endpoint == "" || !s.sourceHostAllowed(endpoint) {
			return false
		}
	}
	if s.gateway != nil && (mode != DirectMode || gatewayStreamType(model.VideoStream{URL: camera.StreamURL, StreamType: camera.StreamType})) {
		return streamOriginAllowed(s.gateway.playbackURL(camera.TenantID, camera.CameraID), allowedOrigins)
	}
	if mode != DirectMode {
		// Vendor SDK URLs are deliberately resolved only during preview because
		// they are short-lived. The preview endpoint re-checks the returned
		// playback origin after resolution; the list endpoint can only expose a
		// provisional eligibility flag here.
		return camera.Enabled && len(allowedOrigins) > 0
	}
	streamType := sourceType(camera.StreamURL, camera.StreamType)
	return (streamType == "hls" || streamType == "mp4" || streamType == "webm" || streamType == "native") && streamOriginAllowed(camera.StreamURL, allowedOrigins)
}

func (s *Service) resolveSource(ctx context.Context, camera model.VideoCameraMapping) (model.VideoStream, error) {
	mode := normalizeMode(camera.IngestMode)
	if mode == DirectMode {
		if strings.TrimSpace(camera.StreamURL) == "" {
			return model.VideoStream{}, errors.New("direct camera stream URL is not configured")
		}
		if err := validateSourceURL(camera.StreamURL); err != nil {
			return model.VideoStream{}, err
		}
		return model.VideoStream{URL: camera.StreamURL, StreamType: sourceType(camera.StreamURL, camera.StreamType), Provider: DirectMode}, nil
	}
	endpoint := s.sdkEndpoint(mode, camera)
	if endpoint == "" {
		return model.VideoStream{}, fmt.Errorf("%s SDK adapter is not configured", mode)
	}
	return s.resolveSDK(ctx, camera, mode, endpoint)
}

func (s *Service) sdkEndpoint(mode string, camera model.VideoCameraMapping) string {
	if endpoint := strings.TrimSpace(camera.SDKEndpoint); endpoint != "" {
		return endpoint
	}
	switch mode {
	case DahuaSDKMode:
		return strings.TrimSpace(s.config.DahuaSDKURL)
	case HikvisionMode:
		return strings.TrimSpace(s.config.HikvisionAPIURL)
	default:
		return ""
	}
}

func (s *Service) resolveSDK(ctx context.Context, camera model.VideoCameraMapping, mode, endpoint string) (model.VideoStream, error) {
	if camera.SDKCameraID == "" {
		return model.VideoStream{}, fmt.Errorf("%s SDK camera ID is required", mode)
	}
	if err := validateSourceEndpoint(endpoint); err != nil {
		return model.VideoStream{}, fmt.Errorf("invalid %s SDK endpoint: %w", mode, err)
	}
	if !s.sourceHostAllowed(endpoint) {
		return model.VideoStream{}, fmt.Errorf("%s SDK endpoint host is not allowlisted", mode)
	}
	if mode == HikvisionMode {
		if s.config.HikvisionResolver == nil {
			return model.VideoStream{}, errors.New("official Hikvision Go adapter is not configured")
		}
		stream, err := s.config.HikvisionResolver.Resolve(ctx, StreamRequest{
			Provider:      mode,
			TenantID:      camera.TenantID,
			CameraID:      camera.SDKCameraID,
			CredentialRef: camera.SDKCredentialRef,
			Endpoint:      endpoint,
		})
		if err != nil {
			return model.VideoStream{}, err
		}
		if err := validateSourceURL(stream.URL); err != nil {
			return model.VideoStream{}, fmt.Errorf("Hikvision SDK returned an invalid stream URL: %w", err)
		}
		stream.Provider = mode
		stream.StreamType = sourceType(stream.URL, stream.StreamType)
		return stream, nil
	}
	statusCode, body, err := s.requestSDK(ctx, camera, mode, endpoint)
	if err != nil {
		return model.VideoStream{}, err
	}
	if statusCode/100 != 2 {
		return model.VideoStream{}, fmt.Errorf("%s SDK returned HTTP %d", mode, statusCode)
	}
	stream, err := decodeSDKStream(body)
	if err != nil {
		return model.VideoStream{}, fmt.Errorf("decode %s SDK response: %w", mode, err)
	}
	stream.Provider = mode
	return stream, nil
}

func (s *Service) requestSDK(ctx context.Context, camera model.VideoCameraMapping, mode, endpoint string) (int, []byte, error) {
	payload, _ := json.Marshal(map[string]any{
		"provider":      mode,
		"cameraId":      camera.SDKCameraID,
		"credentialRef": camera.SDKCredentialRef,
		"tenantId":      camera.TenantID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	token := s.config.DahuaSDKToken
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request %s SDK: %w", mode, err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxSDKResponse+1))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if len(responseBody) > maxSDKResponse {
		return resp.StatusCode, nil, errors.New("video SDK response is too large")
	}
	return resp.StatusCode, responseBody, nil
}

func decodeSDKStream(body []byte) (model.VideoStream, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return model.VideoStream{}, err
	}
	streamURL, streamType, expiresAt, ok := findSDKStream(value, 0)
	if !ok {
		return model.VideoStream{}, errors.New("SDK did not return a stream URL")
	}
	if err := validateSourceURL(streamURL); err != nil {
		return model.VideoStream{}, fmt.Errorf("SDK returned an invalid stream URL: %w", err)
	}
	return model.VideoStream{URL: streamURL, StreamType: sourceType(streamURL, streamType), ExpiresAt: expiresAt}, nil
}

func findSDKStream(value any, depth int) (string, string, int64, bool) {
	if depth > 4 {
		return "", "", 0, false
	}
	switch item := value.(type) {
	case string:
		var nested any
		if json.Unmarshal([]byte(item), &nested) == nil {
			return findSDKStream(nested, depth+1)
		}
	case map[string]any:
		streamURL := firstString(item, "streamUrl", "streamURL", "stream_url", "url", "rtspUrl", "rtspURL", "rtmpUrl", "rtmpURL", "hlsUrl", "hlsURL")
		streamType := firstString(item, "streamType", "stream_type", "protocol")
		expiresAt := firstTimestamp(item, "expiresAt", "expires_at", "expireAt", "expire_at", "expireTime", "expire_time")
		if streamURL != "" {
			return streamURL, streamType, expiresAt, true
		}
		for _, key := range []string{"data", "result", "body"} {
			if nested, exists := item[key]; exists {
				if streamURL, streamType, expiresAt, ok := findSDKStream(nested, depth+1); ok {
					return streamURL, streamType, expiresAt, true
				}
			}
		}
	}
	return "", "", 0, false
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstTimestamp(values map[string]any, keys ...string) int64 {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch timestamp := value.(type) {
		case json.Number:
			if parsed, err := timestamp.Int64(); err == nil {
				return parsed
			}
		case float64:
			return int64(timestamp)
		case string:
			if parsed, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64); err == nil {
				return parsed
			}
			for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05"} {
				if parsed, err := time.Parse(layout, strings.TrimSpace(timestamp)); err == nil {
					return parsed.UnixMilli()
				}
			}
		}
	}
	return 0
}

type zlmGateway struct {
	apiURL       string
	playbackBase string
	secret       string
	vhost        string
	app          string
	client       *http.Client
	mu           sync.Mutex
	proxies      map[string]zlmProxy
}

type zlmProxy struct {
	sourceURL string
	key       string
}

func (g *zlmGateway) proxy(ctx context.Context, tenantID, cameraID string, source model.VideoStream) (model.VideoPreview, error) {
	stream := zlmStreamName(tenantID, cameraID)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.proxies == nil {
		g.proxies = make(map[string]zlmProxy)
	}

	previous, hasPrevious := g.proxies[stream]
	if hasPrevious && previous.sourceURL == source.URL {
		return model.VideoPreview{CameraID: cameraID, PlaybackURL: g.playbackURL(tenantID, cameraID), StreamType: "hls", Provider: source.Provider, ExpiresAt: source.ExpiresAt}, nil
	}
	if hasPrevious && previous.sourceURL != source.URL {
		if err := g.deleteStreamProxy(ctx, previous.key); err != nil {
			return model.VideoPreview{}, err
		}
		delete(g.proxies, stream)
	}

	key, err := g.addStreamProxy(ctx, stream, source.URL)
	if err != nil {
		return model.VideoPreview{}, err
	}
	if key == "" {
		key = zlmProxyKey(g.vhost, g.app, stream)
	}
	g.proxies[stream] = zlmProxy{sourceURL: source.URL, key: key}

	return model.VideoPreview{CameraID: cameraID, PlaybackURL: g.playbackURL(tenantID, cameraID), StreamType: "hls", Provider: source.Provider, ExpiresAt: source.ExpiresAt}, nil
}

func (g *zlmGateway) addStreamProxy(ctx context.Context, stream, sourceURL string) (string, error) {
	form := url.Values{}
	form.Set("secret", g.secret)
	form.Set("vhost", g.vhost)
	form.Set("app", g.app)
	form.Set("stream", stream)
	form.Set("url", sourceURL)
	form.Set("retry_count", "3")
	form.Set("timeout_sec", "10")
	form.Set("enable_hls", "true")
	form.Set("hls_demand", "false")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.apiURL+"/index/api/addStreamProxy", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request ZLMediaKit: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSDKResponse+1))
	if err != nil {
		return "", err
	}
	if len(body) > maxSDKResponse {
		return "", errors.New("ZLMediaKit response is too large")
	}
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode ZLMediaKit response: %w", err)
	}
	if resp.StatusCode/100 != 2 || result.Code != 0 && !proxyAlreadyExists(result.Msg) {
		return "", fmt.Errorf("ZLMediaKit addStreamProxy failed: %s", firstNonEmpty(result.Msg, resp.Status))
	}
	return result.Data.Key, nil
}

func (g *zlmGateway) deleteStreamProxy(ctx context.Context, key string) error {
	if strings.TrimSpace(key) == "" {
		return nil
	}
	form := url.Values{}
	form.Set("secret", g.secret)
	form.Set("key", key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.apiURL+"/index/api/delStreamProxy", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("request ZLMediaKit proxy removal: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSDKResponse+1))
	if err != nil {
		return err
	}
	if len(body) > maxSDKResponse {
		return errors.New("ZLMediaKit response is too large")
	}
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("decode ZLMediaKit removal response: %w", err)
	}
	if resp.StatusCode/100 == 2 && (result.Code == 0 || proxyMissing(result.Msg)) {
		return nil
	}
	return fmt.Errorf("ZLMediaKit delStreamProxy failed: %s", firstNonEmpty(result.Msg, resp.Status))
}

func proxyMissing(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(message, "not found") || strings.Contains(message, "not exist") || strings.Contains(message, "不存在")
}

func proxyAlreadyExists(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(message, "already exist") || strings.Contains(message, "已存在") || strings.Contains(message, "已经存在")
}

func zlmProxyKey(vhost, app, stream string) string {
	return path.Join(vhost, app, stream)
}

func (g *zlmGateway) playbackURL(tenantID, cameraID string) string {
	u, _ := url.Parse(g.playbackBase)
	u.Path = path.Join(u.Path, g.app, zlmStreamName(tenantID, cameraID), "hls.m3u8")
	query := u.Query()
	if g.vhost != "" && g.vhost != "__defaultVhost__" {
		query.Set("vhost", g.vhost)
	}
	u.RawQuery = query.Encode()
	return u.String()
}

func zlmStreamName(tenantID, cameraID string) string {
	hash := sha256.Sum256([]byte(strings.TrimSpace(tenantID) + "\x00" + strings.TrimSpace(cameraID)))
	return "camera_" + hex.EncodeToString(hash[:8])
}

func gatewayStreamType(stream model.VideoStream) bool {
	typ := sourceType(stream.URL, stream.StreamType)
	return typ == "hls" || typ == "rtsp" || typ == "rtmp"
}

func normalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case DahuaSDKMode:
		return DahuaSDKMode
	case HikvisionMode:
		return HikvisionMode
	default:
		return DirectMode
	}
}

func sourceType(rawURL, configured string) string {
	if configured = strings.ToLower(strings.TrimSpace(configured)); configured != "" {
		return configured
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "native"
	}
	switch strings.ToLower(u.Scheme) {
	case "rtsp", "rtmp":
		return strings.ToLower(u.Scheme)
	}
	lower := strings.ToLower(u.Path)
	switch {
	case strings.HasSuffix(lower, ".m3u8") || strings.EqualFold(u.Query().Get("format"), "hls"):
		return "hls"
	case strings.HasSuffix(lower, ".mp4"):
		return "mp4"
	case strings.HasSuffix(lower, ".webm"):
		return "webm"
	default:
		return "native"
	}
}

func validateSourceURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return errors.New("camera stream URL must be an absolute URL")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "rtsp", "rtmp":
		return nil
	default:
		return fmt.Errorf("unsupported camera stream URL scheme %q", u.Scheme)
	}
}

func validateSourceEndpoint(raw string) error {
	u, err := absoluteHTTPURL(raw)
	if err != nil {
		return err
	}
	if u.User != nil {
		return errors.New("endpoint userinfo is not allowed")
	}
	return nil
}

func (s *Service) sourceHostAllowed(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Hostname() == "" || len(s.config.AllowedSourceHosts) == 0 {
		return false
	}
	for _, allowed := range s.config.AllowedSourceHosts {
		if strings.EqualFold(strings.TrimSpace(allowed), u.Hostname()) {
			return true
		}
	}
	return false
}

func absoluteHTTPURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("an absolute HTTP(S) URL is required")
	}
	return u, nil
}

func streamOriginAllowed(rawURL string, allowed []string) bool {
	target, err := absoluteHTTPURL(rawURL)
	if err != nil {
		return false
	}
	targetOrigin := strings.ToLower(target.Scheme + "://" + target.Host)
	for _, candidate := range allowed {
		origin, originErr := absoluteHTTPURL(candidate)
		if originErr == nil && strings.ToLower(origin.Scheme+"://"+origin.Host) == targetOrigin {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
