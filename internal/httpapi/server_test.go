package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVideoSignature(t *testing.T) {
	body := []byte(`{"eventId":"1"}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte("123"))
	_, _ = mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))
	if !verifySignature("secret", "123", body, signature) {
		t.Fatal("valid signature rejected")
	}
	if verifySignature("secret", "123", body, "bad") {
		t.Fatal("bad signature accepted")
	}
}

func TestResolveBrowserStreamType(t *testing.T) {
	tests := []struct {
		name, rawURL, configured, want string
		wantErr                        bool
	}{
		{name: "hls extension", rawURL: "https://media.example/live/cam.m3u8", want: "hls"},
		{name: "hls query", rawURL: "https://media.example/live?id=1&format=hls", want: "hls"},
		{name: "explicit mp4", rawURL: "https://media.example/live?id=1", configured: "mp4", want: "mp4"},
		{name: "rtsp", rawURL: "rtsp://media.example/live/cam", want: "rtsp"},
		{name: "rtsp cannot be hls", rawURL: "rtsp://media.example/live/cam", configured: "hls", wantErr: true},
		{name: "https cannot be rtsp", rawURL: "https://media.example/live/cam", configured: "rtsp", wantErr: true},
		{name: "missing", wantErr: true},
		{name: "bad scheme", rawURL: "file:///tmp/cam.mp4", wantErr: true},
		{name: "bad type", rawURL: "https://media.example/live", configured: "flv", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveBrowserStreamType(tt.rawURL, tt.configured)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("resolveBrowserStreamType() = %q, %v; want %q, error=%v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestAIProviderOriginAllowed(t *testing.T) {
	allowed := []string{"https://api.deepseek.com", "http://localhost:11434", "http://[::1]:11434"}
	tests := []struct {
		name, target string
		want         bool
	}{
		{name: "official API", target: "https://api.deepseek.com/v1", want: true},
		{name: "default HTTPS port", target: "https://api.deepseek.com:443", want: true},
		{name: "exact Ollama port", target: "http://localhost:11434/api", want: true},
		{name: "wrong local port", target: "http://localhost:8080", want: false},
		{name: "userinfo rejected", target: "https://token@api.deepseek.com", want: false},
		{name: "lookalike host", target: "https://api.deepseek.com.attacker.example", want: false},
		{name: "unsupported scheme", target: "file:///tmp/provider", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := originAllowed(tt.target, allowed); got != tt.want {
				t.Fatalf("originAllowed(%q)=%v want=%v", tt.target, got, tt.want)
			}
		})
	}
}

func TestVideoPreviewOriginAllowed(t *testing.T) {
	allowed := []string{"https://media.example", "rtsp://gateway.internal:8554"}
	if !streamOriginAllowed("https://media.example/live/cam.m3u8?token=redacted", allowed) {
		t.Fatal("allowlisted HLS origin was rejected")
	}
	if !streamOriginAllowed("rtsp://gateway.internal:8554/live/cam", allowed) {
		t.Fatal("allowlisted RTSP origin was rejected")
	}
	for _, target := range []string{"http://media.example/live/cam.m3u8", "https://media.example:8443/live/cam.m3u8", "http://127.0.0.1:8080/private"} {
		if streamOriginAllowed(target, allowed) {
			t.Fatalf("non-allowlisted stream origin %q was accepted", target)
		}
	}
}
