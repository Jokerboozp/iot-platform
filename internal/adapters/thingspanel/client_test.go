package thingspanel

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"iot-platform/internal/adapters/memory"
)

func TestSyncUsesThingsPanelTokenHeader(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"tenant_id":"tenant-upstream","authority":"TENANT_ADMIN"}`))
	token := "e30." + payload + ".signature"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/login":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"data":{"token":%q}}`, token)
		case "/api/v1/device":
			if got := r.Header.Get("x-token"); got != token {
				http.Error(w, "missing x-token", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":{"total":1,"list":[{"id":"device-1","name":"Smoke 1","device_config_id":"product-1","device_config_name":"Smoke","activate_flag":"A","device_type":"0"}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	repo := memory.NewRepository()
	client := New(server.URL, "service@example.com", "secret", repo)
	result, err := client.Sync(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Devices != 1 || result.Products != 1 {
		t.Fatalf("unexpected sync result: %+v", result)
	}
	if _, err = repo.GetManagedDevice(context.Background(), "tenant-upstream", "device-1"); err != nil {
		t.Fatalf("synced device missing: %v", err)
	}
}
