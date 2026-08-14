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
