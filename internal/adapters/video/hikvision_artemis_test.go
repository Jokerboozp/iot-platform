package video

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApplyHikvisionSignature(t *testing.T) {
	body := []byte(`{"cameraIndexCode":"camera-001"}`)
	req := httptest.NewRequest(http.MethodPost, "https://hikcentral.example.internal/artemis/api/video/v2/cameras/previewURLs", bytes.NewReader(body))
	applyHikvisionSignature(req, body, "app-key", "app-secret", "Thu, 01 Jan 2026 00:00:00 GMT")
	if req.Header.Get("X-Ca-Key") != "app-key" || req.Header.Get("X-Ca-Signature-Headers") != "x-ca-key,x-ca-nonce,x-ca-timestamp" {
		t.Fatalf("unexpected signing headers: %#v", req.Header)
	}
	nonce := req.Header.Get("X-Ca-Nonce")
	timestamp := req.Header.Get("X-Ca-Timestamp")
	if nonce == "" || timestamp == "" || req.Header.Get("X-Ca-Signature") == "" {
		t.Fatalf("missing generated signing values: %#v", req.Header)
	}
	digest := md5.Sum(body)
	if got, want := req.Header.Get("Content-MD5"), base64.StdEncoding.EncodeToString(digest[:]); got != want {
		t.Fatalf("Content-MD5 = %q, want %q", got, want)
	}
	stringToSign := strings.Join([]string{
		http.MethodPost,
		"*/*",
		base64.StdEncoding.EncodeToString(digest[:]),
		"application/json",
		"Thu, 01 Jan 2026 00:00:00 GMT",
		"x-ca-key:app-key",
		"x-ca-nonce:" + nonce,
		"x-ca-timestamp:" + timestamp,
		req.URL.RequestURI(),
	}, "\n")
	hash := hmac.New(sha256.New, []byte("app-secret"))
	_, _ = hash.Write([]byte(stringToSign))
	wantSignature := base64.StdEncoding.EncodeToString(hash.Sum(nil))
	if got := req.Header.Get("X-Ca-Signature"); got != wantSignature {
		t.Fatalf("signature = %q, want %q", got, wantSignature)
	}
}

func TestParseHikvisionTimestamp(t *testing.T) {
	checks := []struct {
		name string
		raw  string
		want int64
	}{
		{name: "unix seconds", raw: `1740000000`, want: 1740000000000},
		{name: "unix milliseconds", raw: `1740000000000`, want: 1740000000000},
		{name: "rfc3339", raw: `"2025-02-20T10:00:00+08:00"`, want: 1740016800000},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			got := parseHikvisionTimestamp(json.RawMessage(check.raw))
			if got != check.want {
				t.Fatalf("parseHikvisionTimestamp(%s) = %d, want %d", check.raw, got, check.want)
			}
		})
	}
}

func TestHikvisionPreviewURLNormalizesOfficialAPIPath(t *testing.T) {
	got, err := hikvisionPreviewURL("https://hikcentral.example.internal")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://hikcentral.example.internal/artemis/api/video/v2/cameras/previewURLs"
	if got != want {
		t.Fatalf("preview URL = %q, want %q", got, want)
	}
}

func TestHikvisionPreviewURLPreservesExplicitAPIPath(t *testing.T) {
	got, err := hikvisionPreviewURL("https://hikcentral.example.internal/artemis/api/video/v2/cameras/previewURLs")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://hikcentral.example.internal/artemis/api/video/v2/cameras/previewURLs"
	if got != want {
		t.Fatalf("preview URL = %q, want %q", got, want)
	}
}
