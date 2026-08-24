package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"iot-platform/internal/model"
	"iot-platform/internal/ports"
)

var ErrNotFound = errors.New("not found")

type Repository struct {
	mu                sync.RWMutex
	raw               map[string]model.RawArchiveIndex
	standard          map[string]model.StandardMessage
	states            map[string]model.DeviceState
	stateEvents       []model.DeviceState
	rules             map[string]model.AlarmRule
	alarms            map[string]model.Alarm
	video             map[string]model.VideoAlarmEvent
	videoMappings     map[string]model.VideoCameraMapping
	ai                map[string]model.AIAnalysis
	knowledge         map[string]model.KnowledgeDoc
	workflowKnowledge map[string]model.WorkflowKnowledgeBinding
	replays           map[string]model.ReplayRequest
	audits            []model.AuditLog
	aiToolCalls       []model.AIToolCallLog
	products          map[string]model.Product
	protocols         map[string]model.ProtocolPackage
	devices           map[string]model.ManagedDevice
}

func NewRepository() *Repository {
	return &Repository{raw: map[string]model.RawArchiveIndex{}, standard: map[string]model.StandardMessage{}, states: map[string]model.DeviceState{}, rules: map[string]model.AlarmRule{}, alarms: map[string]model.Alarm{}, video: map[string]model.VideoAlarmEvent{}, videoMappings: map[string]model.VideoCameraMapping{}, ai: map[string]model.AIAnalysis{}, knowledge: map[string]model.KnowledgeDoc{}, workflowKnowledge: map[string]model.WorkflowKnowledgeBinding{}, replays: map[string]model.ReplayRequest{}, products: map[string]model.Product{}, protocols: map[string]model.ProtocolPackage{}, devices: map[string]model.ManagedDevice{}}
}

func key(parts ...string) string { return strings.Join(parts, "\x00") }

func (r *Repository) SaveProduct(_ context.Context, v model.Product) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.products[key(v.TenantID, v.ID)] = clone(v)
	return nil
}
func (r *Repository) GetProduct(_ context.Context, tenant, id string) (model.Product, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.products[key(tenant, id)]
	if !ok {
		return v, ErrNotFound
	}
	return clone(v), nil
}
func (r *Repository) ListProducts(_ context.Context, tenant string) ([]model.Product, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []model.Product{}
	for _, v := range r.products {
		if tenant == "" || v.TenantID == tenant {
			out = append(out, clone(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}
func (r *Repository) SaveProtocolPackage(_ context.Context, v model.ProtocolPackage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.protocols[key(v.TenantID, v.ID)] = clone(v)
	return nil
}
func (r *Repository) GetProtocolPackage(_ context.Context, tenant, id string) (model.ProtocolPackage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.protocols[key(tenant, id)]
	if !ok {
		return v, ErrNotFound
	}
	return clone(v), nil
}
func (r *Repository) ListProtocolPackages(_ context.Context, tenant string) ([]model.ProtocolPackage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []model.ProtocolPackage{}
	for _, v := range r.protocols {
		if tenant == "" || v.TenantID == tenant {
			out = append(out, clone(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}
func (r *Repository) SaveManagedDevice(_ context.Context, v model.ManagedDevice) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, current := range r.devices {
		if current.AccessKey == v.AccessKey && (current.TenantID != v.TenantID || current.ID != v.ID) {
			return fmt.Errorf("access key already exists")
		}
	}
	r.devices[key(v.TenantID, v.ID)] = cloneManaged(v)
	return nil
}
func (r *Repository) GetManagedDevice(_ context.Context, tenant, id string) (model.ManagedDevice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.devices[key(tenant, id)]
	if !ok {
		return v, ErrNotFound
	}
	return cloneManaged(v), nil
}
func (r *Repository) GetManagedDeviceByAccessKey(_ context.Context, accessKey string) (model.ManagedDevice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, v := range r.devices {
		if v.AccessKey == accessKey {
			return cloneManaged(v), nil
		}
	}
	return model.ManagedDevice{}, ErrNotFound
}
func (r *Repository) ListManagedDevices(_ context.Context, tenant string) ([]model.ManagedDevice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []model.ManagedDevice{}
	for _, v := range r.devices {
		if tenant == "" || v.TenantID == tenant {
			out = append(out, cloneManaged(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

func (r *Repository) SaveRawIndex(_ context.Context, v model.RawArchiveIndex) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.raw[key(v.TenantID, v.MessageID)]; ok {
		return false, nil
	}
	r.raw[key(v.TenantID, v.MessageID)] = v
	return true, nil
}
func (r *Repository) MarkRawPublished(_ context.Context, tenant, messageID string, publishedAt int64, lastError string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key(tenant, messageID)
	v, ok := r.raw[k]
	if !ok {
		return ErrNotFound
	}
	v.PublishAttempts++
	v.LastPublishError = lastError
	if lastError == "" {
		v.PublishedAt = publishedAt
	}
	r.raw[k] = v
	return nil
}
func (r *Repository) ListPendingRawIndexes(_ context.Context, limit int) ([]model.RawArchiveIndex, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []model.RawArchiveIndex{}
	for _, v := range r.raw {
		if v.PublishedAt == 0 {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ArchivedAt < out[j].ArchivedAt })
	return page(out, 0, limit), nil
}
func (r *Repository) GetRawIndex(_ context.Context, tenant, messageID string) (model.RawArchiveIndex, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.raw[key(tenant, messageID)]
	if !ok {
		return v, ErrNotFound
	}
	return v, nil
}
func (r *Repository) ListRawIndexes(_ context.Context, f ports.RawFilter) ([]model.RawArchiveIndex, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.RawArchiveIndex, 0)
	for _, v := range r.raw {
		if f.TenantID != "" && v.TenantID != f.TenantID || f.ProductID != "" && v.ProductID != f.ProductID || f.DeviceID != "" && v.DeviceID != f.DeviceID || f.Start > 0 && v.ReceivedAt < f.Start || f.End > 0 && v.ReceivedAt > f.End {
			continue
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ReceivedAt > out[j].ReceivedAt })
	return page(out, f.Offset, f.Limit), nil
}
func page[T any](v []T, offset, limit int) []T {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(v) {
		return []T{}
	}
	if limit <= 0 {
		limit = 100
	}
	end := offset + limit
	if end > len(v) {
		end = len(v)
	}
	return v[offset:end]
}
func (r *Repository) SaveStandardMessage(_ context.Context, v model.StandardMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.standard[key(v.TenantID, v.MessageID)] = v
	return nil
}
func (r *Repository) GetStandardMessageByRaw(_ context.Context, tenant, rawID string) (model.StandardMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, v := range r.standard {
		if v.TenantID == tenant && v.RawMessageID == rawID {
			return clone(v), nil
		}
	}
	return model.StandardMessage{}, ErrNotFound
}
func (r *Repository) GetLatestMessage(_ context.Context, tenant, device string) (model.StandardMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out model.StandardMessage
	found := false
	for _, v := range r.standard {
		if v.TenantID == tenant && v.DeviceID == device && (!found || v.Timestamp > out.Timestamp) {
			out, found = clone(v), true
		}
	}
	if !found {
		return out, ErrNotFound
	}
	return out, nil
}
func (r *Repository) PropertyHistory(_ context.Context, tenant, device, property string, start, end int64, limit int) ([]map[string]any, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []map[string]any{}
	for _, v := range r.standard {
		if v.TenantID != tenant || v.DeviceID != device || v.Timestamp < start || (end > 0 && v.Timestamp > end) {
			continue
		}
		value, ok := v.Properties[property]
		if !ok {
			continue
		}
		out = append(out, map[string]any{"timestamp": v.Timestamp, "value": value, "messageId": v.MessageID})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["timestamp"].(int64) < out[j]["timestamp"].(int64) })
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}
func (r *Repository) UpsertDeviceState(_ context.Context, v model.DeviceState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.states[key(v.TenantID, v.DeviceID)] = v
	return nil
}
func (r *Repository) GetDeviceState(_ context.Context, tenant, device string) (model.DeviceState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.states[key(tenant, device)]
	if !ok {
		return v, ErrNotFound
	}
	return v, nil
}
func (r *Repository) ListDeviceStates(_ context.Context, tenant string) ([]model.DeviceState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []model.DeviceState{}
	for _, v := range r.states {
		if tenant == "" || v.TenantID == tenant {
			out = append(out, v)
		}
	}
	return out, nil
}
func (r *Repository) SaveDeviceStateEvent(_ context.Context, v model.DeviceState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stateEvents = append(r.stateEvents, v)
	return nil
}
func (r *Repository) SaveRule(_ context.Context, v model.AlarmRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rules[key(v.TenantID, v.ID)] = v
	return nil
}
func (r *Repository) ListRules(_ context.Context, tenant string) ([]model.AlarmRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []model.AlarmRule{}
	for _, v := range r.rules {
		if tenant == "" || v.TenantID == tenant {
			out = append(out, v)
		}
	}
	return out, nil
}
func (r *Repository) DeleteRule(_ context.Context, tenant, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key(tenant, id)
	if _, ok := r.rules[k]; !ok {
		return ErrNotFound
	}
	delete(r.rules, k)
	return nil
}
func (r *Repository) UpsertAlarm(_ context.Context, v model.Alarm) (model.Alarm, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, a := range r.alarms {
		if a.TenantID == v.TenantID && a.DeviceID == v.DeviceID && a.RuleID == v.RuleID && (a.Status == "ACTIVE" || a.Status == "ACKED") {
			a.LastTriggeredAt = v.LastTriggeredAt
			a.TriggerCount++
			if v.Confidence > a.Confidence {
				a.Confidence = v.Confidence
			}
			r.alarms[k] = cloneAlarm(a)
			return cloneAlarm(a), false, nil
		}
	}
	r.alarms[key(v.TenantID, v.ID)] = cloneAlarm(v)
	return cloneAlarm(v), true, nil
}
func (r *Repository) GetAlarm(_ context.Context, tenant, id string) (model.Alarm, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.alarms[key(tenant, id)]
	if !ok {
		return v, ErrNotFound
	}
	return cloneAlarm(v), nil
}
func (r *Repository) ListAlarms(_ context.Context, f ports.AlarmFilter) ([]model.Alarm, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []model.Alarm{}
	for _, v := range r.alarms {
		if f.TenantID != "" && v.TenantID != f.TenantID || f.DeviceID != "" && v.DeviceID != f.DeviceID || f.Status != "" && v.Status != f.Status || f.Level != "" && v.AlarmLevel != f.Level || f.Source != "" && v.Source != f.Source || f.Start > 0 && v.LastTriggeredAt < f.Start || f.End > 0 && v.LastTriggeredAt > f.End {
			continue
		}
		out = append(out, cloneAlarm(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastTriggeredAt > out[j].LastTriggeredAt })
	return page(out, f.Offset, f.Limit), nil
}
func (r *Repository) UpdateAlarm(_ context.Context, v model.Alarm) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.alarms[key(v.TenantID, v.ID)]; !ok {
		return ErrNotFound
	}
	r.alarms[key(v.TenantID, v.ID)] = cloneAlarm(v)
	return nil
}
func (r *Repository) SaveVideoEvent(_ context.Context, v model.VideoAlarmEvent) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.video[key(v.TenantID, v.EventID)]; ok {
		return false, nil
	}
	r.video[key(v.TenantID, v.EventID)] = cloneVideoEvent(v)
	return true, nil
}
func (r *Repository) UpdateVideoEvent(_ context.Context, v model.VideoAlarmEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.video[key(v.TenantID, v.EventID)] = cloneVideoEvent(v)
	return nil
}
func (r *Repository) ListPendingVideoEvents(_ context.Context, limit int) ([]model.VideoAlarmEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []model.VideoAlarmEvent{}
	for _, v := range r.video {
		status, _ := v.Raw["mediaTransferStatus"].(string)
		if status == "PENDING" || status == "FAILED" {
			out = append(out, cloneVideoEvent(v))
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}
func (r *Repository) SaveVideoCameraMapping(_ context.Context, v model.VideoCameraMapping) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.videoMappings[key(v.TenantID, v.CameraID)] = clone(v)
	return nil
}
func (r *Repository) GetVideoCameraMapping(_ context.Context, tenant, camera string) (model.VideoCameraMapping, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.videoMappings[key(tenant, camera)]
	if !ok {
		return v, ErrNotFound
	}
	return clone(v), nil
}
func (r *Repository) ListVideoCameraMappings(_ context.Context, tenant string) ([]model.VideoCameraMapping, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []model.VideoCameraMapping{}
	for _, v := range r.videoMappings {
		if tenant == "" || v.TenantID == tenant {
			out = append(out, clone(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CameraID < out[j].CameraID })
	return out, nil
}
func (r *Repository) SaveAIAnalysis(_ context.Context, v model.AIAnalysis) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ai[key(v.TenantID, v.AlarmID)] = v
	return nil
}
func (r *Repository) GetAIAnalysis(_ context.Context, tenant, id string) (model.AIAnalysis, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.ai[key(tenant, id)]
	if !ok {
		return v, ErrNotFound
	}
	return v, nil
}
func (r *Repository) SaveKnowledgeDoc(_ context.Context, v model.KnowledgeDoc) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.knowledge[key(v.TenantID, v.ID)] = v
	return nil
}
func (r *Repository) ListKnowledgeDocs(_ context.Context, tenant string) ([]model.KnowledgeDoc, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []model.KnowledgeDoc{}
	for _, v := range r.knowledge {
		if v.TenantID == tenant {
			out = append(out, clone(v))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt == out[j].CreatedAt {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out, nil
}

func (r *Repository) SaveWorkflowKnowledgeBinding(_ context.Context, v model.WorkflowKnowledgeBinding) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workflowKnowledge[key(v.TenantID, v.WorkflowID)] = clone(v)
	return nil
}

func (r *Repository) GetWorkflowKnowledgeBinding(_ context.Context, tenant, workflowID string) (model.WorkflowKnowledgeBinding, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.workflowKnowledge[key(tenant, workflowID)]
	if !ok {
		return model.WorkflowKnowledgeBinding{}, nil
	}
	return clone(v), nil
}
func (r *Repository) SaveReplay(_ context.Context, v model.ReplayRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.replays[v.ID] = v
	return nil
}
func (r *Repository) UpdateReplay(ctx context.Context, v model.ReplayRequest) error {
	return r.SaveReplay(ctx, v)
}
func (r *Repository) GetReplay(_ context.Context, id string) (model.ReplayRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.replays[id]
	if !ok {
		return v, ErrNotFound
	}
	return v, nil
}
func (r *Repository) SaveAudit(_ context.Context, v model.AuditLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.audits = append(r.audits, v)
	return nil
}
func (r *Repository) SaveAIToolCall(_ context.Context, v model.AIToolCallLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.aiToolCalls = append(r.aiToolCalls, clone(v))
	return nil
}
func (r *Repository) Health(context.Context) error { return nil }
func (r *Repository) Close() error                 { return nil }

func cloneAnyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, value := range src {
		switch typed := value.(type) {
		case map[string]any:
			dst[k] = cloneAnyMap(typed)
		case []any:
			items := make([]any, len(typed))
			for i, item := range typed {
				if nested, ok := item.(map[string]any); ok {
					items[i] = cloneAnyMap(nested)
				} else {
					items[i] = item
				}
			}
			dst[k] = items
		case model.VideoAlarmEvent:
			dst[k] = cloneVideoEvent(typed)
		default:
			dst[k] = value
		}
	}
	return dst
}

func cloneVideoEvent(v model.VideoAlarmEvent) model.VideoAlarmEvent {
	v.Raw = cloneAnyMap(v.Raw)
	return v
}

func cloneAlarm(v model.Alarm) model.Alarm {
	v.Details = cloneAnyMap(v.Details)
	return v
}

func clone[T any](v T) T { b, _ := json.Marshal(v); var out T; _ = json.Unmarshal(b, &out); return out }
func cloneManaged(v model.ManagedDevice) model.ManagedDevice {
	if v.Tags != nil {
		v.Tags = clone(v.Tags)
	}
	return v
}

var _ = fmt.Sprintf
