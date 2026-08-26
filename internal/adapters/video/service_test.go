package video

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"iot-platform/internal/model"
)

type staticStreamResolver struct {
	stream model.VideoStream
}

func (r staticStreamResolver) Resolve(context.Context, StreamRequest) (model.VideoStream, error) {
	return r.stream, nil
}

func TestPreviewResolvesSDKURLAndCreatesZLMProxy(t *testing.T) {
	var sdkAuth string
	sdk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sdkAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"streamUrl": "rtsp://camera.internal/live/001", "streamType": "rtsp", "expiresAt": 1730000000000})
	}))
	defer sdk.Close()

	var proxyForm url.Values
	zlm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		proxyForm = r.Form
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "success"})
	}))
	defer zlm.Close()

	service, err := New(Config{ZLMAPIURL: zlm.URL, ZLMPlaybackBaseURL: "https://video.example.internal", ZLMSecret: "zlm-secret", ZLMVhost: "__defaultVhost__", ZLMApp: "iot", DahuaSDKToken: "sdk-token", AllowedSourceHosts: []string{"127.0.0.1", "camera.internal"}})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.Preview(context.Background(), model.VideoCameraMapping{TenantID: "tenant-001", CameraID: "camera-001", CameraName: "一号摄像头", IngestMode: DahuaSDKMode, SDKEndpoint: sdk.URL, SDKCameraID: "channel-001"})
	if err != nil {
		t.Fatal(err)
	}
	if preview.CameraName != "一号摄像头" || preview.StreamType != "hls" || preview.Provider != DahuaSDKMode || preview.PlaybackURL != "https://video.example.internal/iot/"+zlmStreamName("tenant-001", "camera-001")+"/hls.m3u8" {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	if sdkAuth != "Bearer sdk-token" {
		t.Fatalf("SDK authorization = %q", sdkAuth)
	}
	for key, expected := range map[string]string{"secret": "zlm-secret", "app": "iot", "url": "rtsp://camera.internal/live/001", "enable_hls": "true"} {
		if proxyForm.Get(key) != expected {
			t.Fatalf("ZLMediaKit form %s = %q, want %q", key, proxyForm.Get(key), expected)
		}
	}
}

func TestPreviewRejectsBrowserIncompatibleDirectStreamWithoutGateway(t *testing.T) {
	service, err := New(Config{DahuaSDKURL: "http://sdk.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Preview(context.Background(), model.VideoCameraMapping{CameraID: "camera-001", IngestMode: DirectMode, StreamURL: "rtmp://camera.internal/live"})
	if err == nil {
		t.Fatal("expected RTMP preview to require ZLMediaKit")
	}
}

func TestPreviewResolvesHikvisionOfficialArtemisURL(t *testing.T) {
	sdk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/artemis/api/video/v2/cameras/previewURLs" {
			t.Fatalf("unexpected Hikvision request: %s %s", r.Method, r.URL.Path)
		}
		var request struct {
			CameraIndexCode string `json:"cameraIndexCode"`
			Protocol        string `json:"protocol"`
			Transmode       int    `json:"transmode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.CameraIndexCode != "channel-007" || request.Protocol != "rtsp" || request.Transmode != 1 || r.Header.Get("X-Ca-Key") != "hik-key" {
			t.Fatalf("unexpected official Hikvision request: %#v key=%q", request, r.Header.Get("X-Ca-Key"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "0",
			"data": map[string]any{
				"url":        "https://media.example.internal/hikvision/camera-007.m3u8",
				"protocol":   "hls",
				"expireTime": "2025-02-20T10:00:00+08:00",
			},
		})
	}))
	defer sdk.Close()

	resolver, err := NewHikvisionArtemis(HikvisionArtemisConfig{BaseURL: sdk.URL, AppKey: "hik-key", AppSecret: "hik-secret"})
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(Config{HikvisionAPIURL: sdk.URL, HikvisionResolver: resolver, AllowedSourceHosts: []string{"127.0.0.1", "media.example.internal"}})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.Preview(context.Background(), model.VideoCameraMapping{
		TenantID:         "tenant-001",
		CameraID:         "camera-007",
		IngestMode:       HikvisionMode,
		SDKCameraID:      "channel-007",
		SDKCredentialRef: "credential-hik-007",
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.PlaybackURL != "https://media.example.internal/hikvision/camera-007.m3u8" || preview.StreamType != "hls" || preview.Provider != HikvisionMode || preview.ExpiresAt == 0 {
		t.Fatalf("unexpected Hikvision preview: %#v", preview)
	}
}

func TestPreviewOfficialHikvisionRTSPUsesZLMediaKit(t *testing.T) {
	hikcentral := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "0",
			"data": map[string]any{
				"url":      "rtsp://camera.internal/live/official",
				"protocol": "rtsp",
			},
		})
	}))
	defer hikcentral.Close()
	resolver, err := NewHikvisionArtemis(HikvisionArtemisConfig{BaseURL: hikcentral.URL, AppKey: "hik-key", AppSecret: "hik-secret"})
	if err != nil {
		t.Fatal(err)
	}

	var proxySource string
	zlm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		proxySource = r.Form.Get("url")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "success"})
	}))
	defer zlm.Close()

	service, err := New(Config{
		ZLMAPIURL:          zlm.URL,
		ZLMPlaybackBaseURL: "https://video.example.internal",
		ZLMSecret:          "zlm-secret",
		HikvisionAPIURL:    hikcentral.URL,
		HikvisionResolver:  resolver,
		AllowedSourceHosts: []string{"127.0.0.1", "camera.internal"},
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.Preview(context.Background(), model.VideoCameraMapping{
		TenantID:    "tenant-001",
		CameraID:    "camera-official",
		IngestMode:  HikvisionMode,
		SDKCameraID: "channel-official",
	})
	if err != nil {
		t.Fatal(err)
	}
	if proxySource != "rtsp://camera.internal/live/official" || preview.StreamType != "hls" || preview.Provider != HikvisionMode {
		t.Fatalf("unexpected official Hikvision ZLMediaKit preview: source=%q preview=%#v", proxySource, preview)
	}
}

func TestEligibleSDKCameraDefersShortLivedURLResolution(t *testing.T) {
	resolver := staticStreamResolver{stream: model.VideoStream{URL: "https://media.example.internal/live/camera.m3u8", StreamType: "hls"}}
	service, err := New(Config{HikvisionAPIURL: "https://sdk.example.internal/hikvision", HikvisionResolver: resolver, AllowedSourceHosts: []string{"sdk.example.internal"}})
	if err != nil {
		t.Fatal(err)
	}
	camera := model.VideoCameraMapping{Enabled: true, IngestMode: HikvisionMode, SDKCameraID: "channel-007"}
	if !service.Eligible(camera, []string{"https://media.example.internal"}) {
		t.Fatal("SDK camera should remain provisionally preview-eligible until its expiring URL is resolved")
	}
	if service.Eligible(camera, nil) {
		t.Fatal("SDK camera should not be preview-eligible without a browser origin allowlist")
	}
}

func TestPreviewRejectsSDKEndpointOutsideAllowlist(t *testing.T) {
	var contacted atomic.Bool
	sdk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacted.Store(true)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer sdk.Close()

	service, err := New(Config{HikvisionAPIURL: sdk.URL, AllowedSourceHosts: []string{"sdk.example.internal"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Preview(context.Background(), model.VideoCameraMapping{IngestMode: HikvisionMode, SDKCameraID: "channel-007"})
	if err == nil || !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("unexpected disallowed SDK endpoint error: %v", err)
	}
	if contacted.Load() {
		t.Fatal("disallowed SDK endpoint was contacted")
	}
}

func TestPreviewRefreshesZLMProxyWhenSDKURLChanges(t *testing.T) {
	sdkCalls := 0
	sdk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sdkCalls++
		token := "one"
		if sdkCalls > 1 {
			token = "two"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"streamUrl":  "rtsp://camera.internal/live/001?token=" + token,
			"streamType": "rtsp",
		})
	}))
	defer sdk.Close()

	var paths []string
	var forms []url.Values
	zlm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, r.URL.Path)
		forms = append(forms, r.Form)
		if r.URL.Path == "/index/api/addStreamProxy" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]string{"key": "__defaultVhost__/iot/" + r.Form.Get("stream")},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "success"})
	}))
	defer zlm.Close()

	service, err := New(Config{
		ZLMAPIURL:          zlm.URL,
		ZLMPlaybackBaseURL: "https://video.example.internal",
		ZLMSecret:          "zlm-secret",
		DahuaSDKToken:      "sdk-token",
		AllowedSourceHosts: []string{"127.0.0.1", "camera.internal"},
	})
	if err != nil {
		t.Fatal(err)
	}
	camera := model.VideoCameraMapping{
		TenantID:    "tenant-001",
		CameraID:    "camera-001",
		IngestMode:  DahuaSDKMode,
		SDKEndpoint: sdk.URL,
		SDKCameraID: "channel-001",
	}
	if _, err := service.Preview(context.Background(), camera); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Preview(context.Background(), camera); err != nil {
		t.Fatal(err)
	}

	if got, want := len(paths), 3; got != want {
		t.Fatalf("ZLMediaKit call count = %d, want %d (%v)", got, want, paths)
	}
	wantPaths := []string{"/index/api/addStreamProxy", "/index/api/delStreamProxy", "/index/api/addStreamProxy"}
	for i, want := range wantPaths {
		if paths[i] != want {
			t.Fatalf("ZLMediaKit call %d = %q, want %q", i, paths[i], want)
		}
	}
	if got, want := forms[1].Get("key"), "__defaultVhost__/iot/"+zlmStreamName("tenant-001", "camera-001"); got != want {
		t.Fatalf("deleted proxy key = %q, want %q", got, want)
	}
	if got, want := forms[2].Get("url"), "rtsp://camera.internal/live/001?token=two"; got != want {
		t.Fatalf("refreshed proxy URL = %q, want %q", got, want)
	}
}

func TestZLMStreamNameIsTenantScoped(t *testing.T) {
	if first, second := zlmStreamName("tenant-001", "camera-001"), zlmStreamName("tenant-002", "camera-001"); first == second {
		t.Fatalf("tenant-scoped stream names collided: %q", first)
	}
}
