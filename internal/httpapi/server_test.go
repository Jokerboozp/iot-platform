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
