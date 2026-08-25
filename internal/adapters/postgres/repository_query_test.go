package postgres

import (
	"strings"
	"testing"
)

func TestCountManagedDeviceChildrenQueryMatchesDeviceRegistrySchema(t *testing.T) {
	query := strings.ToLower(countManagedDeviceChildrenSQL)
	if !strings.Contains(query, "body->>'gatewayid'") || strings.Contains(query, "device_registry.gateway_id") || strings.Contains(query, " and gateway_id") {
		t.Fatalf("device child count query must read gatewayId from body jsonb, not a physical gateway_id column: %s", countManagedDeviceChildrenSQL)
	}
}
