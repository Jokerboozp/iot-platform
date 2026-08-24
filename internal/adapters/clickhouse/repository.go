package clickhouse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"iot-platform/internal/model"
	"iot-platform/internal/ports"
)

// Repository decorates durable business storage with ClickHouse telemetry writes and queries.
type Repository struct {
	ports.Repository
	base string
	http *http.Client
}

var propertyCodePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func New(ctx context.Context, base string, repo ports.Repository) (*Repository, error) {
	r := &Repository{Repository: repo, base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 30 * time.Second}}
	u, err := url.Parse(r.base)
	if err != nil {
		return nil, err
	}
	database := u.Query().Get("database")
	if database != "" {
		if !propertyCodePattern.MatchString(database) {
			return nil, fmt.Errorf("invalid clickhouse database name %q", database)
		}
		bootstrapURL := *u
		params := bootstrapURL.Query()
		params.Del("database")
		bootstrapURL.RawQuery = params.Encode()
		bootstrap := &Repository{Repository: repo, base: strings.TrimRight(bootstrapURL.String(), "/"), http: r.http}
		if _, err = bootstrap.query(ctx, "CREATE DATABASE IF NOT EXISTS "+database, nil); err != nil {
			return nil, err
		}
	}
	schema := `CREATE TABLE IF NOT EXISTS iot_telemetry (tenant_id String, device_id String, product_id String, message_id String, ts DateTime64(3), properties JSON) ENGINE=MergeTree PARTITION BY toYYYYMM(ts) ORDER BY (tenant_id,device_id,ts,message_id)`
	if _, err = r.query(ctx, schema, nil); err != nil {
		return nil, err
	}
	return r, nil
}
func (r *Repository) SaveStandardMessage(ctx context.Context, v model.StandardMessage) error {
	_, err := r.SaveStandardMessageIfAbsent(ctx, v)
	return err
}
func (r *Repository) SaveStandardMessageIfAbsent(ctx context.Context, v model.StandardMessage) (bool, error) {
	created, err := r.Repository.SaveStandardMessageIfAbsent(ctx, v)
	if err != nil || !created {
		return created, err
	}
	if v.MessageType != model.PropertyReport && v.MessageType != model.AlarmReport {
		return true, nil
	}
	row := map[string]any{"tenant_id": v.TenantID, "device_id": v.DeviceID, "product_id": v.ProductID, "message_id": v.MessageID, "ts": time.UnixMilli(v.Timestamp).UTC().Format("2006-01-02 15:04:05.000"), "properties": v.Properties}
	b, _ := json.Marshal(row)
	b = append(b, '\n')
	_, err = r.query(ctx, "INSERT INTO iot_telemetry FORMAT JSONEachRow", b)
	return true, err
}
func (r *Repository) ClaimStandardMessage(ctx context.Context, v model.StandardMessage) (bool, bool, error) {
	shouldProcess, created, err := r.Repository.ClaimStandardMessage(ctx, v)
	if err != nil || !shouldProcess || !created {
		return shouldProcess, created, err
	}
	if v.MessageType != model.PropertyReport && v.MessageType != model.AlarmReport {
		return true, true, nil
	}
	row := map[string]any{"tenant_id": v.TenantID, "device_id": v.DeviceID, "product_id": v.ProductID, "message_id": v.MessageID, "ts": time.UnixMilli(v.Timestamp).UTC().Format("2006-01-02 15:04:05.000"), "properties": v.Properties}
	b, _ := json.Marshal(row)
	b = append(b, '\n')
	_, err = r.query(ctx, "INSERT INTO iot_telemetry FORMAT JSONEachRow", b)
	return true, true, err
}
func (r *Repository) PropertyHistory(ctx context.Context, tenant, device, property string, start, end int64, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 1000
	}
	if end <= 0 {
		end = time.Now().UnixMilli()
	}
	if !propertyCodePattern.MatchString(property) {
		return r.Repository.PropertyHistory(ctx, tenant, device, property, start, end, limit)
	}
	safeProperty := property
	q := fmt.Sprintf(`SELECT toUnixTimestamp64Milli(ts) AS timestamp, properties.%s AS value, message_id AS messageId FROM iot_telemetry WHERE tenant_id=%s AND device_id=%s AND ts >= fromUnixTimestamp64Milli(%d) AND ts <= fromUnixTimestamp64Milli(%d) ORDER BY ts DESC LIMIT %d FORMAT JSONEachRow`, safeProperty, quote(tenant), quote(device), start, end, limit)
	b, err := r.query(ctx, q, nil)
	if err != nil {
		return r.Repository.PropertyHistory(ctx, tenant, device, property, start, end, limit)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	out := []map[string]any{}
	for i := len(lines) - 1; i >= 0; i-- {
		var v map[string]any
		if lines[i] != "" && json.Unmarshal([]byte(lines[i]), &v) == nil {
			out = append(out, v)
		}
	}
	return out, nil
}
func (r *Repository) query(ctx context.Context, q string, body []byte) ([]byte, error) {
	u, err := url.Parse(r.base)
	if err != nil {
		return nil, err
	}
	params := u.Query()
	params.Set("query", q)
	u.RawQuery = params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("clickhouse %s: %s", resp.Status, string(out))
	}
	return out, nil
}
func quote(v string) string { return "'" + strings.ReplaceAll(v, "'", "''") + "'" }
