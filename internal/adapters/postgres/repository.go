package postgres

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"iot-platform/internal/model"
	"iot-platform/internal/ports"
)

//go:embed schema.sql
var schema string

var ErrNotFound = errors.New("not found")

type Repository struct{ pool *pgxpool.Pool }

func New(ctx context.Context, dsn string) (*Repository, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	r := &Repository{pool: pool}
	if err = r.Migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return r, nil
}
func (r *Repository) Migrate(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, schema)
	return err
}
func (r *Repository) SaveProduct(ctx context.Context, v model.Product) error {
	b, _ := json.Marshal(v)
	_, err := r.pool.Exec(ctx, `INSERT INTO iot_product(tenant_id,id,status,protocol_package_id,body) VALUES($1,$2,$3,$4,$5) ON CONFLICT(tenant_id,id) DO UPDATE SET status=excluded.status,protocol_package_id=excluded.protocol_package_id,body=excluded.body,updated_at=now()`, v.TenantID, v.ID, v.Status, v.ProtocolPackageID, b)
	return err
}
func (r *Repository) GetProduct(ctx context.Context, tenant, id string) (model.Product, error) {
	var v model.Product
	var b []byte
	err := r.pool.QueryRow(ctx, `SELECT body FROM iot_product WHERE tenant_id=$1 AND id=$2`, tenant, id).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(b, &v)
	}
	return v, err
}
func (r *Repository) ListProducts(ctx context.Context, tenant string) ([]model.Product, error) {
	rows, err := r.pool.Query(ctx, `SELECT body FROM iot_product WHERE tenant_id=$1 ORDER BY updated_at DESC`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Product{}
	for rows.Next() {
		var b []byte
		var v model.Product
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) SaveProtocolPackage(ctx context.Context, v model.ProtocolPackage) error {
	b, _ := json.Marshal(v)
	_, err := r.pool.Exec(ctx, `INSERT INTO protocol_package(tenant_id,id,status,parser_type,body) VALUES($1,$2,$3,$4,$5) ON CONFLICT(tenant_id,id) DO UPDATE SET status=excluded.status,parser_type=excluded.parser_type,body=excluded.body,updated_at=now()`, v.TenantID, v.ID, v.Status, v.ParserType, b)
	return err
}
func (r *Repository) GetProtocolPackage(ctx context.Context, tenant, id string) (model.ProtocolPackage, error) {
	var v model.ProtocolPackage
	var b []byte
	err := r.pool.QueryRow(ctx, `SELECT body FROM protocol_package WHERE tenant_id=$1 AND id=$2`, tenant, id).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(b, &v)
	}
	return v, err
}
func (r *Repository) ListProtocolPackages(ctx context.Context, tenant string) ([]model.ProtocolPackage, error) {
	rows, err := r.pool.Query(ctx, `SELECT body FROM protocol_package WHERE tenant_id=$1 ORDER BY updated_at DESC`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ProtocolPackage{}
	for rows.Next() {
		var b []byte
		var v model.ProtocolPackage
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) SaveManagedDevice(ctx context.Context, v model.ManagedDevice) error {
	b, _ := json.Marshal(v)
	_, err := r.pool.Exec(ctx, `INSERT INTO device_registry(tenant_id,id,product_id,status,access_key,secret_hash,body) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(tenant_id,id) DO UPDATE SET product_id=excluded.product_id,status=excluded.status,access_key=excluded.access_key,secret_hash=excluded.secret_hash,body=excluded.body,updated_at=now()`, v.TenantID, v.ID, v.ProductID, v.Status, v.AccessKey, v.SecretHash, b)
	return err
}
func (r *Repository) scanManagedDevice(row pgx.Row) (model.ManagedDevice, error) {
	var v model.ManagedDevice
	var b []byte
	var hash string
	err := row.Scan(&b, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	err = json.Unmarshal(b, &v)
	v.SecretHash = hash
	return v, err
}
func (r *Repository) GetManagedDevice(ctx context.Context, tenant, id string) (model.ManagedDevice, error) {
	return r.scanManagedDevice(r.pool.QueryRow(ctx, `SELECT body,secret_hash FROM device_registry WHERE tenant_id=$1 AND id=$2`, tenant, id))
}
func (r *Repository) GetManagedDeviceByAccessKey(ctx context.Context, accessKey string) (model.ManagedDevice, error) {
	return r.scanManagedDevice(r.pool.QueryRow(ctx, `SELECT body,secret_hash FROM device_registry WHERE access_key=$1`, accessKey))
}
func (r *Repository) ListManagedDevices(ctx context.Context, tenant string) ([]model.ManagedDevice, error) {
	rows, err := r.pool.Query(ctx, `SELECT body,secret_hash FROM device_registry WHERE tenant_id=$1 ORDER BY updated_at DESC`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ManagedDevice{}
	for rows.Next() {
		var v model.ManagedDevice
		var b []byte
		if err = rows.Scan(&b, &v.SecretHash); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) SaveRawIndex(ctx context.Context, v model.RawArchiveIndex) (bool, error) {
	tag, err := r.pool.Exec(ctx, `INSERT INTO raw_archive_index(tenant_id,product_id,device_id,message_id,protocol,payload_format,object_bucket,object_key,object_offset,payload_hash,payload_size,received_at,archived_at,published_at,publish_attempts,last_publish_error) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) ON CONFLICT DO NOTHING`, v.TenantID, v.ProductID, v.DeviceID, v.MessageID, v.Protocol, v.PayloadFormat, v.ObjectBucket, v.ObjectKey, v.ObjectOffset, v.PayloadHash, v.PayloadSize, v.ReceivedAt, v.ArchivedAt, v.PublishedAt, v.PublishAttempts, v.LastPublishError)
	return tag.RowsAffected() == 1, err
}
func (r *Repository) MarkRawPublished(ctx context.Context, tenant, messageID string, publishedAt int64, lastError string) error {
	_, err := r.pool.Exec(ctx, `UPDATE raw_archive_index SET publish_attempts=publish_attempts+1,last_publish_error=$4,published_at=CASE WHEN $4='' THEN $3 ELSE published_at END WHERE tenant_id=$1 AND message_id=$2`, tenant, messageID, publishedAt, lastError)
	return err
}
func (r *Repository) ListPendingRawIndexes(ctx context.Context, limit int) ([]model.RawArchiveIndex, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `SELECT message_id,tenant_id,product_id,device_id,protocol,payload_format,object_bucket,object_key,object_offset,payload_hash,payload_size,received_at,archived_at,published_at,publish_attempts,last_publish_error FROM raw_archive_index WHERE published_at=0 ORDER BY archived_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.RawArchiveIndex{}
	for rows.Next() {
		var v model.RawArchiveIndex
		if err = scanRawIndex(rows, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type rowScanner interface{ Scan(...any) error }

func scanRawIndex(row rowScanner, v *model.RawArchiveIndex) error {
	return row.Scan(&v.MessageID, &v.TenantID, &v.ProductID, &v.DeviceID, &v.Protocol, &v.PayloadFormat, &v.ObjectBucket, &v.ObjectKey, &v.ObjectOffset, &v.PayloadHash, &v.PayloadSize, &v.ReceivedAt, &v.ArchivedAt, &v.PublishedAt, &v.PublishAttempts, &v.LastPublishError)
}
func (r *Repository) GetRawIndex(ctx context.Context, tenant, messageID string) (model.RawArchiveIndex, error) {
	var v model.RawArchiveIndex
	err := scanRawIndex(r.pool.QueryRow(ctx, `SELECT message_id,tenant_id,product_id,device_id,protocol,payload_format,object_bucket,object_key,object_offset,payload_hash,payload_size,received_at,archived_at,published_at,publish_attempts,last_publish_error FROM raw_archive_index WHERE tenant_id=$1 AND message_id=$2`, tenant, messageID), &v)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}
func (r *Repository) ListRawIndexes(ctx context.Context, f ports.RawFilter) ([]model.RawArchiveIndex, error) {
	q := `SELECT message_id,tenant_id,product_id,device_id,protocol,payload_format,object_bucket,object_key,object_offset,payload_hash,payload_size,received_at,archived_at,published_at,publish_attempts,last_publish_error FROM raw_archive_index WHERE ($1='' OR tenant_id=$1) AND ($2='' OR product_id=$2) AND ($3='' OR device_id=$3) AND ($4::bigint=0 OR received_at >= $4) AND ($5::bigint=0 OR received_at <= $5) ORDER BY received_at DESC LIMIT $6 OFFSET $7`
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, q, f.TenantID, f.ProductID, f.DeviceID, f.Start, f.End, limit, f.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.RawArchiveIndex{}
	for rows.Next() {
		var v model.RawArchiveIndex
		if err := scanRawIndex(rows, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) SaveStandardMessage(ctx context.Context, v model.StandardMessage) error {
	body, _ := json.Marshal(v)
	props, _ := json.Marshal(v.Properties)
	event, _ := json.Marshal(v.Event)
	tags, _ := json.Marshal(v.Tags)
	_, err := r.pool.Exec(ctx, `INSERT INTO standard_message(tenant_id,message_id,raw_message_id,product_id,device_id,message_type,ts,properties,event,tags,body) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT DO NOTHING`, v.TenantID, v.MessageID, v.RawMessageID, v.ProductID, v.DeviceID, v.MessageType, v.Timestamp, props, event, tags, body)
	return err
}
func (r *Repository) getStandard(ctx context.Context, query string, args ...any) (model.StandardMessage, error) {
	var v model.StandardMessage
	var body []byte
	err := r.pool.QueryRow(ctx, query, args...).Scan(&body)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(body, &v)
	}
	return v, err
}
func (r *Repository) GetStandardMessageByRaw(ctx context.Context, tenant, rawID string) (model.StandardMessage, error) {
	return r.getStandard(ctx, `SELECT body FROM standard_message WHERE tenant_id=$1 AND raw_message_id=$2 ORDER BY ts DESC LIMIT 1`, tenant, rawID)
}
func (r *Repository) GetLatestMessage(ctx context.Context, tenant, device string) (model.StandardMessage, error) {
	return r.getStandard(ctx, `SELECT body FROM standard_message WHERE tenant_id=$1 AND device_id=$2 ORDER BY ts DESC LIMIT 1`, tenant, device)
}
func (r *Repository) PropertyHistory(ctx context.Context, tenant, device, property string, start, end int64, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := r.pool.Query(ctx, `SELECT ts, properties -> $3, message_id FROM standard_message WHERE tenant_id=$1 AND device_id=$2 AND properties ? $3 AND ts >= $4::bigint AND ($5::bigint=0 OR ts <= $5) ORDER BY ts DESC LIMIT $6`, tenant, device, property, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var ts int64
		var raw []byte
		var id string
		if err := rows.Scan(&ts, &raw, &id); err != nil {
			return nil, err
		}
		var value any
		_ = json.Unmarshal(raw, &value)
		out = append(out, map[string]any{"timestamp": ts, "value": value, "messageId": id})
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}
func (r *Repository) UpsertDeviceState(ctx context.Context, v model.DeviceState) error {
	b, _ := json.Marshal(v)
	_, err := r.pool.Exec(ctx, `INSERT INTO device_state(tenant_id,device_id,product_id,business_status,last_seen_at,body) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(tenant_id,device_id) DO UPDATE SET product_id=excluded.product_id,business_status=excluded.business_status,last_seen_at=excluded.last_seen_at,body=excluded.body,updated_at=now()`, v.TenantID, v.DeviceID, v.ProductID, v.BusinessStatus, v.LastSeenAt, b)
	return err
}
func (r *Repository) GetDeviceState(ctx context.Context, tenant, device string) (model.DeviceState, error) {
	var v model.DeviceState
	var b []byte
	err := r.pool.QueryRow(ctx, `SELECT body FROM device_state WHERE tenant_id=$1 AND device_id=$2`, tenant, device).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(b, &v)
	}
	return v, err
}
func (r *Repository) ListDeviceStates(ctx context.Context, tenant string) ([]model.DeviceState, error) {
	rows, err := r.pool.Query(ctx, `SELECT body FROM device_state WHERE ($1='' OR tenant_id=$1)`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.DeviceState{}
	for rows.Next() {
		var b []byte
		var v model.DeviceState
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) SaveDeviceStateEvent(ctx context.Context, v model.DeviceState) error {
	b, _ := json.Marshal(v)
	_, err := r.pool.Exec(ctx, `INSERT INTO device_state_event(tenant_id,device_id,business_status,body) VALUES($1,$2,$3,$4)`, v.TenantID, v.DeviceID, v.BusinessStatus, b)
	return err
}
func (r *Repository) SaveRule(ctx context.Context, v model.AlarmRule) error {
	b, _ := json.Marshal(v)
	_, err := r.pool.Exec(ctx, `INSERT INTO alarm_rule(tenant_id,id,product_id,enabled,body) VALUES($1,$2,$3,$4,$5) ON CONFLICT(tenant_id,id) DO UPDATE SET product_id=excluded.product_id,enabled=excluded.enabled,body=excluded.body,updated_at=now()`, v.TenantID, v.ID, v.ProductID, v.Enabled, b)
	return err
}
func (r *Repository) ListRules(ctx context.Context, tenant string) ([]model.AlarmRule, error) {
	rows, err := r.pool.Query(ctx, `SELECT body FROM alarm_rule WHERE ($1='' OR tenant_id=$1) ORDER BY updated_at DESC`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.AlarmRule{}
	for rows.Next() {
		var b []byte
		var v model.AlarmRule
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) DeleteRule(ctx context.Context, tenant, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM alarm_rule WHERE tenant_id=$1 AND id=$2`, tenant, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
func (r *Repository) UpsertAlarm(ctx context.Context, v model.Alarm) (model.Alarm, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return v, false, err
	}
	defer tx.Rollback(ctx)
	var body []byte
	err = tx.QueryRow(ctx, `SELECT body FROM alarm_record WHERE tenant_id=$1 AND device_id=$2 AND rule_id=$3 AND status IN ('ACTIVE','ACKED') FOR UPDATE`, v.TenantID, v.DeviceID, v.RuleID).Scan(&body)
	if err == nil {
		var old model.Alarm
		if err = json.Unmarshal(body, &old); err != nil {
			return v, false, err
		}
		old.LastTriggeredAt = v.LastTriggeredAt
		old.TriggerCount++
		if v.Confidence > old.Confidence {
			old.Confidence = v.Confidence
		}
		body, _ = json.Marshal(old)
		_, err = tx.Exec(ctx, `UPDATE alarm_record SET last_triggered_at=$3,body=$4 WHERE tenant_id=$1 AND id=$2`, old.TenantID, old.ID, old.LastTriggeredAt, body)
		if err != nil {
			return v, false, err
		}
		if err = tx.Commit(ctx); err != nil {
			return v, false, err
		}
		return old, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return v, false, err
	}
	body, _ = json.Marshal(v)
	_, err = tx.Exec(ctx, `INSERT INTO alarm_record(tenant_id,id,rule_id,device_id,status,level,source,last_triggered_at,body) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, v.TenantID, v.ID, v.RuleID, v.DeviceID, v.Status, v.AlarmLevel, v.Source, v.LastTriggeredAt, body)
	if err != nil {
		return v, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return v, false, err
	}
	return v, true, nil
}
func (r *Repository) GetAlarm(ctx context.Context, tenant, id string) (model.Alarm, error) {
	var v model.Alarm
	var b []byte
	err := r.pool.QueryRow(ctx, `SELECT body FROM alarm_record WHERE tenant_id=$1 AND id=$2`, tenant, id).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(b, &v)
	}
	return v, err
}
func (r *Repository) ListAlarms(ctx context.Context, f ports.AlarmFilter) ([]model.Alarm, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `SELECT body FROM alarm_record WHERE ($1='' OR tenant_id=$1) AND ($2='' OR device_id=$2) AND ($3='' OR status=$3) AND ($4='' OR level=$4) AND ($5='' OR source=$5) AND ($6::bigint=0 OR last_triggered_at >= $6) AND ($7::bigint=0 OR last_triggered_at <= $7) ORDER BY last_triggered_at DESC LIMIT $8 OFFSET $9`, f.TenantID, f.DeviceID, f.Status, f.Level, f.Source, f.Start, f.End, limit, f.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Alarm{}
	for rows.Next() {
		var b []byte
		var v model.Alarm
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) UpdateAlarm(ctx context.Context, v model.Alarm) error {
	b, _ := json.Marshal(v)
	tag, err := r.pool.Exec(ctx, `UPDATE alarm_record SET status=$3,level=$4,last_triggered_at=$5,body=$6 WHERE tenant_id=$1 AND id=$2`, v.TenantID, v.ID, v.Status, v.AlarmLevel, v.LastTriggeredAt, b)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
func (r *Repository) SaveVideoEvent(ctx context.Context, v model.VideoAlarmEvent) (bool, error) {
	b, _ := json.Marshal(v)
	tag, err := r.pool.Exec(ctx, `INSERT INTO video_alarm_event(tenant_id,event_id,camera_id,alarm_type,event_time,body) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, v.TenantID, v.EventID, v.CameraID, v.AlarmType, v.EventTime, b)
	return tag.RowsAffected() == 1, err
}
func (r *Repository) UpdateVideoEvent(ctx context.Context, v model.VideoAlarmEvent) error {
	b, _ := json.Marshal(v)
	_, err := r.pool.Exec(ctx, `UPDATE video_alarm_event SET body=$3,alarm_type=$4,event_time=$5 WHERE tenant_id=$1 AND event_id=$2`, v.TenantID, v.EventID, b, v.AlarmType, v.EventTime)
	return err
}
func (r *Repository) ListPendingVideoEvents(ctx context.Context, limit int) ([]model.VideoAlarmEvent, error) {
	rows, err := r.pool.Query(ctx, `SELECT body FROM video_alarm_event WHERE body->'raw'->>'mediaTransferStatus' IN ('PENDING','FAILED') ORDER BY event_time LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.VideoAlarmEvent{}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		var v model.VideoAlarmEvent
		if err = json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) SaveVideoCameraMapping(ctx context.Context, v model.VideoCameraMapping) error {
	related, _ := json.Marshal(v.RelatedDeviceIDs)
	_, err := r.pool.Exec(ctx, `INSERT INTO video_camera_mapping(tenant_id,camera_id,camera_name,project_id,city_code,district_code,building,floor,area_id,related_device_ids,video_platform_id,stream_url,enabled) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT(tenant_id,camera_id) DO UPDATE SET camera_name=excluded.camera_name,project_id=excluded.project_id,city_code=excluded.city_code,district_code=excluded.district_code,building=excluded.building,floor=excluded.floor,area_id=excluded.area_id,related_device_ids=excluded.related_device_ids,video_platform_id=excluded.video_platform_id,stream_url=excluded.stream_url,enabled=excluded.enabled`, v.TenantID, v.CameraID, v.CameraName, v.ProjectID, v.CityCode, v.DistrictCode, v.Building, v.Floor, v.AreaID, related, v.VideoPlatformID, v.StreamURL, v.Enabled)
	return err
}
func (r *Repository) scanVideoMapping(row rowScanner) (model.VideoCameraMapping, error) {
	var v model.VideoCameraMapping
	var related []byte
	err := row.Scan(&v.TenantID, &v.CameraID, &v.CameraName, &v.ProjectID, &v.CityCode, &v.DistrictCode, &v.Building, &v.Floor, &v.AreaID, &related, &v.VideoPlatformID, &v.StreamURL, &v.Enabled)
	if err == nil {
		_ = json.Unmarshal(related, &v.RelatedDeviceIDs)
	}
	return v, err
}
func (r *Repository) GetVideoCameraMapping(ctx context.Context, tenant, camera string) (model.VideoCameraMapping, error) {
	v, err := r.scanVideoMapping(r.pool.QueryRow(ctx, `SELECT tenant_id,camera_id,coalesce(camera_name,''),coalesce(project_id,''),coalesce(city_code,''),coalesce(district_code,''),coalesce(building,''),coalesce(floor,''),coalesce(area_id,''),related_device_ids,coalesce(video_platform_id,''),coalesce(stream_url,''),enabled FROM video_camera_mapping WHERE tenant_id=$1 AND camera_id=$2`, tenant, camera))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}
func (r *Repository) ListVideoCameraMappings(ctx context.Context, tenant string) ([]model.VideoCameraMapping, error) {
	rows, err := r.pool.Query(ctx, `SELECT tenant_id,camera_id,coalesce(camera_name,''),coalesce(project_id,''),coalesce(city_code,''),coalesce(district_code,''),coalesce(building,''),coalesce(floor,''),coalesce(area_id,''),related_device_ids,coalesce(video_platform_id,''),coalesce(stream_url,''),enabled FROM video_camera_mapping WHERE tenant_id=$1 ORDER BY camera_id`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.VideoCameraMapping{}
	for rows.Next() {
		v, e := r.scanVideoMapping(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) SaveAIAnalysis(ctx context.Context, v model.AIAnalysis) error {
	b, _ := json.Marshal(v)
	_, err := r.pool.Exec(ctx, `INSERT INTO alarm_ai_analysis(alarm_id,body) VALUES($1,$2) ON CONFLICT(alarm_id) DO UPDATE SET body=excluded.body,created_at=now()`, v.AlarmID, b)
	return err
}
func (r *Repository) GetAIAnalysis(ctx context.Context, id string) (model.AIAnalysis, error) {
	var v model.AIAnalysis
	var b []byte
	err := r.pool.QueryRow(ctx, `SELECT body FROM alarm_ai_analysis WHERE alarm_id=$1`, id).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(b, &v)
	}
	return v, err
}
func (r *Repository) SaveKnowledgeDoc(ctx context.Context, v model.KnowledgeDoc) error {
	b, _ := json.Marshal(v.Metadata)
	_, err := r.pool.Exec(ctx, `INSERT INTO ai_knowledge_doc(id,tenant_id,product_id,object_bucket,object_key,filename,status,metadata) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(id) DO UPDATE SET status=excluded.status,metadata=excluded.metadata`, v.ID, v.TenantID, v.ProductID, v.ObjectBucket, v.ObjectKey, v.Filename, v.Status, b)
	return err
}
func (r *Repository) SaveReplay(ctx context.Context, v model.ReplayRequest) error {
	b, _ := json.Marshal(v)
	_, err := r.pool.Exec(ctx, `INSERT INTO replay_task(id,tenant_id,status,body) VALUES($1,$2,$3,$4)`, v.ID, v.TenantID, v.Status, b)
	return err
}
func (r *Repository) UpdateReplay(ctx context.Context, v model.ReplayRequest) error {
	b, _ := json.Marshal(v)
	_, err := r.pool.Exec(ctx, `UPDATE replay_task SET status=$2,body=$3,updated_at=now() WHERE id=$1`, v.ID, v.Status, b)
	return err
}
func (r *Repository) GetReplay(ctx context.Context, id string) (model.ReplayRequest, error) {
	var v model.ReplayRequest
	var b []byte
	err := r.pool.QueryRow(ctx, `SELECT body FROM replay_task WHERE id=$1`, id).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(b, &v)
	}
	return v, err
}
func (r *Repository) SaveAudit(ctx context.Context, v model.AuditLog) error {
	b, _ := json.Marshal(v.Details)
	_, err := r.pool.Exec(ctx, `INSERT INTO audit_log(id,tenant_id,actor,action,target_type,target_id,details,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, v.ID, v.TenantID, v.Actor, v.Action, v.TargetType, v.TargetID, b, v.CreatedAt)
	return err
}
func (r *Repository) SaveAIToolCall(ctx context.Context, v model.AIToolCallLog) error {
	in, _ := json.Marshal(v.Input)
	out, _ := json.Marshal(v.Output)
	if v.Error != "" {
		out, _ = json.Marshal(map[string]any{"error": v.Error, "output": v.Output})
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO ai_tool_call_log(tenant_id,actor,tool,input,output,success,created_at) VALUES($1,$2,$3,$4,$5,$6,to_timestamp($7::double precision/1000))`, v.TenantID, v.Actor, v.Tool, in, out, v.Success, v.CreatedAt)
	return err
}
func (r *Repository) Health(ctx context.Context) error { return r.pool.Ping(ctx) }
func (r *Repository) Close() error                     { r.pool.Close(); return nil }

var _ = fmt.Sprintf
var _ = strings.Builder{}
