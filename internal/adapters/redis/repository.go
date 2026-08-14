package redisadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"iot-platform/internal/model"
	"iot-platform/internal/ports"
)

// Repository decorates the durable repository with Redis-backed hot device state.
type Repository struct {
	ports.Repository
	client *redis.Client
}

func New(base ports.Repository, addr, password string) *Repository {
	return &Repository{Repository: base, client: redis.NewClient(&redis.Options{Addr: addr, Password: password})}
}
func stateKey(tenant, device string) string { return fmt.Sprintf("device:state:%s:%s", tenant, device) }
func latestKey(tenant, device string) string {
	return fmt.Sprintf("device:latest:%s:%s", tenant, device)
}
func (r *Repository) UpsertDeviceState(ctx context.Context, v model.DeviceState) error {
	if err := r.Repository.UpsertDeviceState(ctx, v); err != nil {
		return err
	}
	b, _ := json.Marshal(v)
	pipe := r.client.TxPipeline()
	pipe.Set(ctx, stateKey(v.TenantID, v.DeviceID), b, 0)
	if v.BusinessStatus == "ONLINE" {
		pipe.SAdd(ctx, "device:online:"+v.TenantID, v.DeviceID)
	} else {
		pipe.SRem(ctx, "device:online:"+v.TenantID, v.DeviceID)
	}
	_, err := pipe.Exec(ctx)
	return err
}
func (r *Repository) SaveStandardMessage(ctx context.Context, v model.StandardMessage) error {
	if err := r.Repository.SaveStandardMessage(ctx, v); err != nil {
		return err
	}
	b, _ := json.Marshal(v)
	return r.client.Set(ctx, latestKey(v.TenantID, v.DeviceID), b, 7*24*time.Hour).Err()
}
func (r *Repository) GetLatestMessage(ctx context.Context, tenant, device string) (model.StandardMessage, error) {
	var v model.StandardMessage
	b, err := r.client.Get(ctx, latestKey(tenant, device)).Bytes()
	if err == nil && json.Unmarshal(b, &v) == nil {
		return v, nil
	}
	return r.Repository.GetLatestMessage(ctx, tenant, device)
}
func alarmKey(v model.Alarm) string {
	return fmt.Sprintf("alarm:active:%s:%s:%s", v.TenantID, v.DeviceID, v.RuleID)
}
func (r *Repository) UpsertAlarm(ctx context.Context, v model.Alarm) (model.Alarm, bool, error) {
	a, created, err := r.Repository.UpsertAlarm(ctx, v)
	if err != nil {
		return a, created, err
	}
	b, _ := json.Marshal(a)
	if cacheErr := r.client.Set(ctx, alarmKey(a), b, 0).Err(); cacheErr != nil {
		return a, created, cacheErr
	}
	return a, created, nil
}
func (r *Repository) UpdateAlarm(ctx context.Context, v model.Alarm) error {
	if err := r.Repository.UpdateAlarm(ctx, v); err != nil {
		return err
	}
	if v.Status == "ACTIVE" || v.Status == "ACKED" {
		b, _ := json.Marshal(v)
		return r.client.Set(ctx, alarmKey(v), b, 0).Err()
	}
	return r.client.Del(ctx, alarmKey(v)).Err()
}
func (r *Repository) GetDeviceState(ctx context.Context, tenant, device string) (model.DeviceState, error) {
	var v model.DeviceState
	b, err := r.client.Get(ctx, stateKey(tenant, device)).Bytes()
	if err == nil && json.Unmarshal(b, &v) == nil {
		return v, nil
	}
	return r.Repository.GetDeviceState(ctx, tenant, device)
}
func (r *Repository) Health(ctx context.Context) error {
	if err := r.Repository.Health(ctx); err != nil {
		return err
	}
	return r.client.Ping(ctx).Err()
}
func (r *Repository) Close() error { return errors.Join(r.Repository.Close(), r.client.Close()) }
