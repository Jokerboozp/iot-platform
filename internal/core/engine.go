package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"iot-platform/internal/model"
	"iot-platform/internal/parser"
	"iot-platform/internal/ports"
)

type Engine struct {
	Repo                      ports.Repository
	Archive                   ports.Archive
	Bus                       ports.EventBus
	Realtime                  ports.RealtimePublisher
	AI                        ports.AIClient
	AIPlugins                 ports.AIPluginRegistry
	AIWorkflows               ports.AIWorkflowRuntime
	KB                        ports.KnowledgeBase
	Catalog                   ports.PlatformCatalog
	Parsers                   *parser.Registry
	Clock                     ports.Clock
	Log                       *slog.Logger
	Metrics                   interface{ Inc(string) }
	VideoMediaAllowedHosts    []string
	RequireVideoCameraMapping bool
	VideoPreview              ports.VideoPreviewService
}

func New(repo ports.Repository, archive ports.Archive, bus ports.EventBus, realtime ports.RealtimePublisher, parsers *parser.Registry, log *slog.Logger) *Engine {
	return &Engine{Repo: repo, Archive: archive, Bus: bus, Realtime: realtime, Parsers: parsers, Clock: ports.RealClock{}, Log: log}
}
func (e *Engine) Start(ctx context.Context) error {
	subs := []struct {
		topic, group string
		h            ports.Handler
	}{{model.TopicRaw, "parser", e.handleRaw}, {model.TopicPropertyReport, "storage", e.handleStandard}, {model.TopicEventReport, "storage", e.handleStandard}, {model.TopicDeviceState, "state", e.handleState}, {model.TopicAlarmRaised, "ai", e.handleAI}}
	for _, s := range subs {
		if err := e.Bus.Subscribe(ctx, s.topic, s.group, s.h); err != nil {
			return err
		}
	}
	go e.retryPendingRaw(ctx)
	go e.retryPendingVideoMedia(ctx)
	return nil
}
func (e *Engine) IngestRaw(ctx context.Context, raw model.RawMessage) (model.RawArchiveIndex, bool, error) {
	raw.Normalize(e.Clock.Now())
	if err := raw.Validate(); err != nil {
		return model.RawArchiveIndex{}, false, err
	}
	if err := e.ensureGatewayChild(ctx, raw); err != nil {
		return model.RawArchiveIndex{}, false, err
	}
	if existing, err := e.Repo.GetRawIndex(ctx, raw.TenantID, raw.MessageID); err == nil {
		if existing.PublishedAt == 0 {
			stored, readErr := e.Archive.GetRaw(ctx, existing)
			if readErr != nil {
				return existing, false, fmt.Errorf("read pending raw: %w", readErr)
			}
			if publishErr := e.publishArchivedRaw(ctx, existing, stored); publishErr != nil {
				return existing, false, publishErr
			}
		}
		return existing, false, nil
	}
	idx, err := e.Archive.PutRaw(ctx, raw)
	if err != nil {
		if e.Metrics != nil {
			e.Metrics.Inc("raw_archive_failed_total")
		}
		return idx, false, fmt.Errorf("archive raw: %w", err)
	}
	created, err := e.Repo.SaveRawIndex(ctx, idx)
	if err != nil {
		return idx, false, fmt.Errorf("index raw: %w", err)
	}
	if !created {
		return idx, false, nil
	}
	if e.Metrics != nil {
		e.Metrics.Inc("mqtt_ingest_qps")
		e.Metrics.Inc("raw_archive_success_total")
	}
	if err := e.publishArchivedRaw(ctx, idx, raw); err != nil {
		return idx, true, err
	}
	return idx, true, nil
}

func (e *Engine) publishArchivedRaw(ctx context.Context, idx model.RawArchiveIndex, raw model.RawMessage) error {
	b, _ := json.Marshal(raw)
	if err := e.Bus.Publish(ctx, model.TopicRaw, raw.DeviceID, b); err != nil {
		_ = e.Repo.MarkRawPublished(ctx, idx.TenantID, idx.MessageID, 0, err.Error())
		if e.Metrics != nil {
			e.Metrics.Inc("raw_publish_failed_total")
		}
		return fmt.Errorf("publish archived raw: %w", err)
	}
	if err := e.Repo.MarkRawPublished(ctx, idx.TenantID, idx.MessageID, e.Clock.Now().UnixMilli(), ""); err != nil {
		return fmt.Errorf("mark raw published: %w", err)
	}
	return nil
}

func (e *Engine) retryPendingRaw(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			indexes, err := e.Repo.ListPendingRawIndexes(ctx, 200)
			if err != nil {
				e.Log.Error("list pending raw", "error", err)
				continue
			}
			for _, idx := range indexes {
				raw, readErr := e.Archive.GetRaw(ctx, idx)
				if readErr != nil {
					_ = e.Repo.MarkRawPublished(ctx, idx.TenantID, idx.MessageID, 0, readErr.Error())
					continue
				}
				if publishErr := e.publishArchivedRaw(ctx, idx, raw); publishErr != nil {
					e.Log.Warn("retry pending raw", "messageId", idx.MessageID, "error", publishErr)
				}
			}
		}
	}
}

func (e *Engine) ensureGatewayChild(ctx context.Context, raw model.RawMessage) error {
	if raw.GatewayID == "" || raw.DeviceID == raw.GatewayID {
		return nil
	}
	gateway, err := e.Repo.GetManagedDevice(ctx, raw.TenantID, raw.GatewayID)
	if err != nil {
		return fmt.Errorf("gateway %s is not registered", raw.GatewayID)
	}
	gatewayProduct, err := e.Repo.GetProduct(ctx, raw.TenantID, gateway.ProductID)
	if err != nil {
		return fmt.Errorf("gateway product not found")
	}
	if gateway.DeviceRole != "GATEWAY" && gatewayProduct.Category != "gateway" {
		return fmt.Errorf("device %s is not configured as a gateway", gateway.ID)
	}
	if raw.ProductID == "" {
		return fmt.Errorf("child productId is required")
	}
	childProduct, err := e.Repo.GetProduct(ctx, raw.TenantID, raw.ProductID)
	if err != nil || childProduct.Status != "ENABLED" {
		return fmt.Errorf("child product is not enabled")
	}
	if child, childErr := e.Repo.GetManagedDevice(ctx, raw.TenantID, raw.DeviceID); childErr == nil {
		if child.DeviceRole != "CHILD" || child.GatewayID != gateway.ID {
			return fmt.Errorf("device %s is already registered outside gateway %s", child.ID, gateway.ID)
		}
		if child.ProductID != raw.ProductID {
			return fmt.Errorf("child device product does not match its registration")
		}
		return nil
	}
	now := e.Clock.Now().UnixMilli()
	secret := id("child_secret")
	hash := sha256.Sum256([]byte(secret))
	child := model.ManagedDevice{
		ID:                 raw.DeviceID,
		TenantID:           raw.TenantID,
		ProductID:          raw.ProductID,
		Name:               raw.DeviceName,
		Status:             "ENABLED",
		DeviceRole:         "CHILD",
		GatewayID:          gateway.ID,
		RegistrationSource: "GATEWAY_AUTO",
		AutoRegistered:     true,
		AccessKey:          "dk_" + strings.TrimPrefix(id(""), "_"),
		SecretHash:         hex.EncodeToString(hash[:]),
		SecretHint:         "网关托管",
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if child.Name == "" {
		child.Name = "子设备 " + raw.DeviceID
	}
	if err = e.Repo.SaveManagedDevice(ctx, child); err != nil {
		return fmt.Errorf("auto-register child device: %w", err)
	}
	_ = e.Repo.SaveAudit(ctx, model.AuditLog{ID: id("audit"), TenantID: raw.TenantID, Actor: "gateway:" + gateway.ID, Action: "device.child.auto-register", TargetType: "device", TargetID: child.ID, Details: map[string]any{"gatewayId": gateway.ID, "productId": child.ProductID}, CreatedAt: now})
	return nil
}
func (e *Engine) handleRaw(ctx context.Context, b []byte) error {
	var raw model.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	var msg *model.StandardMessage
	var err error
	product, productErr := e.Repo.GetProduct(ctx, raw.TenantID, raw.ProductID)
	if productErr == nil && product.ProtocolPackageID != "" {
		pkg, pkgErr := e.Repo.GetProtocolPackage(ctx, raw.TenantID, product.ProtocolPackageID)
		if pkgErr == nil && pkg.Status == "PUBLISHED" {
			msg, err = e.Parsers.ParseVersionWithConfig(pkg.ParserType, raw.ParserVersion, pkg.Config, raw)
		}
	}
	if msg == nil && err == nil {
		msg, err = e.Parsers.Parse(raw)
	}
	if err != nil {
		if e.Metrics != nil {
			e.Metrics.Inc("parse_failed_total")
		}
		failure, _ := json.Marshal(map[string]any{"raw": raw, "stage": "parse", "error": err.Error(), "failedAt": e.Clock.Now().UnixMilli()})
		_ = e.Bus.Publish(ctx, model.TopicParseFailed, raw.MessageID, failure)
		return nil
	}
	out, _ := json.Marshal(msg)
	topic := model.TopicPropertyReport
	switch msg.MessageType {
	case model.EventReport, model.AlarmReport:
		topic = model.TopicEventReport
	}
	return e.Bus.Publish(ctx, topic, msg.DeviceID, out)
}
func (e *Engine) handleStandard(ctx context.Context, b []byte) error {
	var msg model.StandardMessage
	if err := json.Unmarshal(b, &msg); err != nil {
		return err
	}
	shouldProcess, _, err := e.Repo.ClaimStandardMessage(ctx, msg)
	if err != nil {
		return err
	}
	if !shouldProcess {
		return nil
	}
	if err := e.touchState(ctx, msg); err != nil {
		return err
	}
	rules, err := e.Repo.ListRules(ctx, msg.TenantID)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if MatchRule(rule, msg) {
			if rule.DurationSeconds > 0 {
				satisfied, durationErr := e.durationSatisfied(ctx, rule, msg)
				if durationErr != nil {
					return durationErr
				}
				if !satisfied {
					continue
				}
			}
			if _, _, err := e.raiseRuleAlarm(ctx, rule, msg); err != nil {
				return err
			}
		} else if MatchConditions(rule.Recovery, msg) {
			if err := e.clearDuration(ctx, rule, msg); err != nil {
				return err
			}
			if err := e.recoverRuleAlarm(ctx, rule, msg); err != nil {
				return err
			}
		} else {
			if err := e.clearDuration(ctx, rule, msg); err != nil {
				return err
			}
		}
	}
	return e.Repo.MarkStandardMessageProcessed(ctx, msg.TenantID, msg.MessageID)
}

func (e *Engine) clearDuration(ctx context.Context, rule model.AlarmRule, msg model.StandardMessage) error {
	return e.Repo.DeleteRulePending(ctx, rule.TenantID, rule.ID, msg.DeviceID)
}
func (e *Engine) durationSatisfied(ctx context.Context, rule model.AlarmRule, msg model.StandardMessage) (bool, error) {
	now := e.Clock.Now().Unix()
	since, found, err := e.Repo.GetRulePending(ctx, rule.TenantID, rule.ID, msg.DeviceID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, e.Repo.SaveRulePending(ctx, rule.TenantID, rule.ID, msg.DeviceID, now)
	}
	return now-since >= rule.DurationSeconds, nil
}
func (e *Engine) touchState(ctx context.Context, msg model.StandardMessage) error {
	state, err := e.Repo.GetDeviceState(ctx, msg.TenantID, msg.DeviceID)
	if err != nil {
		state = model.DeviceState{TenantID: msg.TenantID, ProductID: msg.ProductID, DeviceID: msg.DeviceID, ReportIntervalSec: 300, OfflineToleranceSec: 60, ConnectionStatus: "UNKNOWN"}
	}
	old := state.BusinessStatus
	state.DataStatus = "ACTIVE"
	state.BusinessStatus = "ONLINE"
	state.LastSeenAt = msg.Timestamp
	state.LastMessageID = msg.MessageID
	state.StatusSource = "RAW_MESSAGE"
	if err := e.Repo.UpsertDeviceState(ctx, state); err != nil {
		return err
	}
	if old != state.BusinessStatus {
		_ = e.Repo.SaveDeviceStateEvent(ctx, state)
		payload, _ := json.Marshal(state)
		_ = e.Realtime.Publish(ctx, fmt.Sprintf("/iot/device/state/%s/%s/%s", state.TenantID, state.ProductID, state.DeviceID), payload, 1, true)
	}
	return nil
}
func (e *Engine) raiseRuleAlarm(ctx context.Context, rule model.AlarmRule, msg model.StandardMessage) (model.Alarm, bool, error) {
	now := e.Clock.Now().UnixMilli()
	a := model.Alarm{ID: id("alarm"), TenantID: msg.TenantID, RuleID: rule.ID, TriggerID: msg.MessageID, DeviceID: msg.DeviceID, AlarmType: rule.AlarmType, AlarmLevel: rule.Level, Status: "ACTIVE", Source: "device", CityCode: tag(msg, "cityCode", "unknown"), DistrictCode: tag(msg, "districtCode", "unknown"), BuildingID: tag(msg, "buildingId", "unknown"), DeviceType: tag(msg, "deviceType", msg.ProductID), AreaID: tag(msg, "areaId", ""), FirstTriggeredAt: now, LastTriggeredAt: now, TriggerCount: 1, Details: map[string]any{"message": msg, "ruleName": rule.Name}}
	saved, created, err := e.Repo.UpsertAlarm(ctx, a)
	if err != nil {
		return saved, false, err
	}
	if created {
		if e.Metrics != nil {
			e.Metrics.Inc("alarm_trigger_total")
		}
		payload, _ := json.Marshal(saved)
		_ = e.Bus.Publish(ctx, model.TopicAlarmRaised, saved.ID, payload)
		_ = e.Realtime.Publish(ctx, saved.MQTTTopic("raised"), payload, 1, false)
	}
	// Alarm records are deduplicated while ACTIVE/ACKED, but a new matching
	// message must still execute the rule actions. Exact duplicate messages
	// keep the original trigger ID and must not execute actions twice.
	if created || saved.TriggerID != msg.MessageID {
		for _, action := range rule.Actions {
			event := model.UIActionEvent{ID: id("ui_action"), TenantID: msg.TenantID, RuleID: rule.ID, AlarmID: saved.ID, DeviceID: msg.DeviceID, Action: action, TriggeredAt: now}
			actionPayload, _ := json.Marshal(event)
			_ = e.Bus.Publish(ctx, model.TopicUIAction, event.ID, actionPayload)
			_ = e.Realtime.Publish(ctx, fmt.Sprintf("/iot/ui-action/%s", msg.TenantID), actionPayload, 1, false)
		}
	}
	return saved, created, nil
}
func (e *Engine) recoverRuleAlarm(ctx context.Context, rule model.AlarmRule, msg model.StandardMessage) error {
	alarms, err := e.Repo.ListAlarms(ctx, ports.AlarmFilter{TenantID: msg.TenantID, DeviceID: msg.DeviceID, Status: "ACTIVE", Limit: 100})
	if err != nil {
		return err
	}
	for _, a := range alarms {
		if a.RuleID != rule.ID {
			continue
		}
		a.Status = "RECOVERED"
		a.RecoveredAt = e.Clock.Now().UnixMilli()
		if err := e.Repo.UpdateAlarm(ctx, a); err != nil {
			return err
		}
		payload, _ := json.Marshal(a)
		_ = e.Bus.Publish(ctx, model.TopicAlarmRecovered, a.ID, payload)
		_ = e.Realtime.Publish(ctx, a.MQTTTopic("recovered"), payload, 1, false)
	}
	return nil
}

// DeleteRule removes a rule and closes any alarms that can no longer be
// recovered by the deleted rule. Historical alarm rows are retained.
func (e *Engine) DeleteRule(ctx context.Context, tenant, ruleID string) error {
	if err := e.Repo.DeleteRule(ctx, tenant, ruleID); err != nil {
		return err
	}
	if err := e.Repo.DeleteRulePendings(ctx, tenant, ruleID); err != nil {
		return err
	}
	return e.closeRuleAlarms(ctx, tenant, ruleID)
}

// DisableRule clears duration state and closes active/acknowledged alarms
// before a rule is switched off. Historical alarm rows remain available.
func (e *Engine) DisableRule(ctx context.Context, tenant, ruleID string) error {
	if err := e.Repo.DeleteRulePendings(ctx, tenant, ruleID); err != nil {
		return err
	}
	return e.closeRuleAlarms(ctx, tenant, ruleID)
}

func (e *Engine) closeRuleAlarms(ctx context.Context, tenant, ruleID string) error {
	const batchSize = 1000
	for _, status := range []string{"ACTIVE", "ACKED"} {
		offset := 0
		for {
			alarms, err := e.Repo.ListAlarms(ctx, ports.AlarmFilter{TenantID: tenant, Status: status, Limit: batchSize, Offset: offset})
			if err != nil {
				return err
			}
			if len(alarms) == 0 {
				break
			}
			changed := false
			for _, alarm := range alarms {
				if alarm.RuleID != ruleID {
					continue
				}
				alarm.Status = "RECOVERED"
				alarm.RecoveredAt = e.Clock.Now().UnixMilli()
				if err := e.Repo.UpdateAlarm(ctx, alarm); err != nil {
					return err
				}
				payload := mustJSON(alarm)
				_ = e.Bus.Publish(ctx, model.TopicAlarmRecovered, alarm.ID, payload)
				_ = e.Realtime.Publish(ctx, alarm.MQTTTopic("recovered"), payload, 1, false)
				changed = true
			}
			if changed {
				// Updating rows removes them from the status-filtered result set;
				// restart at zero so offset pagination cannot skip the next row.
				offset = 0
				continue
			}
			offset += len(alarms)
		}
	}
	return nil
}
func (e *Engine) handleState(ctx context.Context, b []byte) error {
	var state model.DeviceState
	if err := json.Unmarshal(b, &state); err == nil && state.DeviceID != "" {
		return e.UpdateDeviceState(ctx, state)
	}
	var msg model.StandardMessage
	if err := json.Unmarshal(b, &msg); err != nil {
		return err
	}
	return e.touchState(ctx, msg)
}
func (e *Engine) UpdateDeviceState(ctx context.Context, state model.DeviceState) error {
	old, _ := e.Repo.GetDeviceState(ctx, state.TenantID, state.DeviceID)
	if state.LastSeenAt == 0 {
		state.LastSeenAt = old.LastSeenAt
	}
	if err := e.Repo.UpsertDeviceState(ctx, state); err != nil {
		return err
	}
	if old.BusinessStatus != state.BusinessStatus {
		_ = e.Repo.SaveDeviceStateEvent(ctx, state)
		b, _ := json.Marshal(state)
		return e.Realtime.Publish(ctx, fmt.Sprintf("/iot/device/state/%s/%s/%s", state.TenantID, state.ProductID, state.DeviceID), b, 1, true)
	}
	return nil
}
func (e *Engine) ScanOffline(ctx context.Context) error {
	states, err := e.Repo.ListDeviceStates(ctx, "")
	if err != nil {
		return err
	}
	now := e.Clock.Now().UnixMilli()
	for _, s := range states {
		deadline := s.LastSeenAt + (s.ReportIntervalSec+s.OfflineToleranceSec)*1000
		if s.LastSeenAt == 0 || deadline >= now {
			continue
		}
		s.DataStatus = "SILENT"
		if s.ConnectionStatus == "CONNECTED" {
			s.BusinessStatus = "SUSPECTED_OFFLINE"
		} else {
			s.BusinessStatus = "OFFLINE"
		}
		s.OfflineAt = deadline
		s.OfflineDetectedAt = now
		s.StatusSource = "RAW_MESSAGE_TIMEOUT"
		_ = e.UpdateDeviceState(ctx, s)
	}
	return nil
}
func (e *Engine) IngestVideo(ctx context.Context, v model.VideoAlarmEvent) (model.Alarm, bool, error) {
	if v.EventID == "" || v.TenantID == "" || v.CameraID == "" || v.AlarmType == "" {
		return model.Alarm{}, false, errors.New("eventId, tenantId, cameraId and alarmType are required")
	}
	if v.ReceivedAt == 0 {
		v.ReceivedAt = e.Clock.Now().UnixMilli()
	}
	mapping, mappingErr := e.Repo.GetVideoCameraMapping(ctx, v.TenantID, v.CameraID)
	if mappingErr != nil && e.RequireVideoCameraMapping {
		return model.Alarm{}, false, fmt.Errorf("camera %s is not bound to tenant %s", v.CameraID, v.TenantID)
	}
	if mappingErr == nil {
		if !mapping.Enabled {
			return model.Alarm{}, false, fmt.Errorf("camera %s is disabled", v.CameraID)
		}
		if v.CameraName == "" {
			v.CameraName = mapping.CameraName
		}
		if v.ProjectID == "" {
			v.ProjectID = mapping.ProjectID
		}
		if v.AreaID == "" {
			v.AreaID = mapping.AreaID
		}
		if v.CityCode == "" {
			v.CityCode = mapping.CityCode
		}
		if v.DistrictCode == "" {
			v.DistrictCode = mapping.DistrictCode
		}
		if v.BuildingID == "" {
			v.BuildingID = mapping.Building
		}
	}
	if err := e.validateVideoMediaURLs(v); err != nil {
		return model.Alarm{}, false, err
	}
	if v.Raw == nil {
		v.Raw = map[string]any{}
	}
	if isExternalMedia(v.SnapshotURL) || isExternalMedia(v.VideoClipURL) {
		v.Raw["mediaTransferStatus"] = "PENDING"
	}
	created, err := e.Repo.SaveVideoEvent(ctx, v)
	if err != nil || !created {
		if err != nil && e.Metrics != nil {
			e.Metrics.Inc("video_alarm_failed_total")
		}
		return model.Alarm{}, false, err
	}
	if e.Metrics != nil {
		e.Metrics.Inc("video_alarm_ingest_total")
	}
	_ = e.Bus.Publish(ctx, model.TopicVideoAlarm, v.CameraID, mustJSON(v))
	now := e.Clock.Now().UnixMilli()
	a := model.Alarm{ID: id("alarm"), TenantID: v.TenantID, RuleID: "video:" + v.AlarmType, TriggerID: v.EventID, DeviceID: v.CameraID, DeviceName: v.CameraName, AlarmType: v.AlarmType, AlarmLevel: v.AlarmLevel, Status: "ACTIVE", Source: "video", CityCode: v.CityCode, DistrictCode: v.DistrictCode, BuildingID: v.BuildingID, DeviceType: "video_ai", AreaID: v.AreaID, FirstTriggeredAt: now, LastTriggeredAt: now, TriggerCount: 1, Confidence: v.Confidence, Details: map[string]any{"videoEvent": v}}
	if a.AlarmLevel == "" {
		a.AlarmLevel = map[bool]string{true: "MEDIUM", false: "HIGH"}[v.Confidence < 0.6]
	}
	if v.Confidence < 0.6 {
		a.Details["requiresVerification"] = true
	}
	if fused, ok, fuseErr := e.fuseVideoAlarm(ctx, a); fuseErr != nil {
		return model.Alarm{}, false, fuseErr
	} else if ok {
		return fused, false, nil
	}
	saved, isNew, err := e.Repo.UpsertAlarm(ctx, a)
	if err == nil && isNew {
		payload := mustJSON(saved)
		_ = e.Bus.Publish(ctx, model.TopicAlarmRaised, saved.ID, payload)
		_ = e.Realtime.Publish(ctx, saved.MQTTTopic("raised"), payload, 1, false)
	}
	if v.Raw["mediaTransferStatus"] == "PENDING" {
		go e.processVideoMedia(context.Background(), v)
	}
	return saved, isNew, err
}

func (e *Engine) fuseVideoAlarm(ctx context.Context, incoming model.Alarm) (model.Alarm, bool, error) {
	active, err := e.Repo.ListAlarms(ctx, ports.AlarmFilter{TenantID: incoming.TenantID, Status: "ACTIVE", Limit: 1000})
	if err != nil {
		return incoming, false, err
	}
	windowStart := incoming.LastTriggeredAt - 120_000
	for _, existing := range active {
		if existing.LastTriggeredAt < windowStart || existing.AreaID == "" || incoming.AreaID == "" || existing.AreaID != incoming.AreaID {
			continue
		}
		if existing.Source == "video" && existing.AlarmType == incoming.AlarmType {
			existing.LastTriggeredAt = incoming.LastTriggeredAt
			existing.TriggerCount++
			if incoming.Confidence > existing.Confidence {
				existing.Confidence = incoming.Confidence
			}
			if existing.Details == nil {
				existing.Details = map[string]any{}
			}
			existing.Details["latestVideoEvent"] = incoming.Details["videoEvent"]
			if err = e.Repo.UpdateAlarm(ctx, existing); err != nil {
				return incoming, false, err
			}
			payload := mustJSON(existing)
			_ = e.Realtime.Publish(ctx, existing.MQTTTopic("raised"), payload, 1, false)
			return existing, true, nil
		}
		if existing.Source != "video" && relatedFireAlarm(existing.AlarmType, incoming.AlarmType) {
			incoming.MultiSource = true
			existing.MultiSource = true
			if incoming.AlarmLevel != "CRITICAL" {
				incoming.AlarmLevel = "CRITICAL"
			}
			if existing.AlarmLevel != "CRITICAL" {
				existing.AlarmLevel = "CRITICAL"
			}
			if existing.Details == nil {
				existing.Details = map[string]any{}
			}
			existing.Details["videoConfirmation"] = incoming.Details["videoEvent"]
			_ = e.Repo.UpdateAlarm(ctx, existing)
		}
	}
	return incoming, false, nil
}
func relatedFireAlarm(deviceType, videoType string) bool {
	a := strings.ToUpper(deviceType)
	b := strings.ToUpper(videoType)
	return (strings.Contains(a, "FIRE") || strings.Contains(a, "SMOKE") || strings.Contains(a, "TEMPERATURE")) && (strings.Contains(b, "FIRE") || strings.Contains(b, "FLAME") || strings.Contains(b, "SMOKE"))
}
func (e *Engine) handleAI(ctx context.Context, b []byte) error {
	if e.AI == nil {
		return nil
	}
	var alarm model.Alarm
	if err := json.Unmarshal(b, &alarm); err != nil {
		return err
	}
	_, err := e.AnalyzeAlarm(ctx, alarm.TenantID, alarm.ID)
	return err
}

// AnalyzeAlarm runs the same automatic analysis path used by alarm events and
// is also exposed to the operator UI for a manual re-run.
func (e *Engine) AnalyzeAlarm(ctx context.Context, tenantID, alarmID string) (model.AIAnalysis, error) {
	if e.AI == nil {
		return model.AIAnalysis{}, errors.New("AI model is not configured")
	}
	alarm, err := e.Repo.GetAlarm(ctx, tenantID, alarmID)
	if err != nil {
		return model.AIAnalysis{}, err
	}
	history := []map[string]any{}
	if device, deviceErr := e.Repo.GetManagedDevice(ctx, alarm.TenantID, alarm.DeviceID); deviceErr == nil {
		history = append(history, map[string]any{"contextType": "deviceMetadata", "device": device})
	}
	properties := []string{"temperature", "smoke", "water_pressure", "voltage", "current", "gas"}
	for _, property := range properties {
		for _, window := range []struct {
			name  string
			ms    int64
			limit int
		}{{"10m", 10 * 60 * 1000, 200}, {"1h", 60 * 60 * 1000, 500}, {"24h", 24 * 60 * 60 * 1000, 1000}} {
			items, historyErr := e.Repo.PropertyHistory(ctx, alarm.TenantID, alarm.DeviceID, property, alarm.LastTriggeredAt-window.ms, alarm.LastTriggeredAt, window.limit)
			if historyErr == nil && len(items) > 0 {
				history = append(history, map[string]any{"contextType": "propertyHistory", "property": property, "window": window.name, "items": items})
			}
		}
	}
	if similar, similarErr := e.Repo.ListAlarms(ctx, ports.AlarmFilter{TenantID: alarm.TenantID, DeviceID: alarm.DeviceID, Limit: 20}); similarErr == nil {
		filtered := []model.Alarm{}
		for _, item := range similar {
			if item.ID != alarm.ID && item.AlarmType == alarm.AlarmType {
				filtered = append(filtered, item)
			}
		}
		history = append(history, map[string]any{"contextType": "similarAlarms", "items": filtered})
	}
	knowledge := []string{}
	if e.KB != nil {
		knowledge, _ = e.KB.Search(ctx, alarm.TenantID, strings.Join([]string{alarm.AlarmType, alarm.DeviceType, "处置 SOP 维修"}, " "), 8)
	}
	analysis, err := e.AI.AnalyzeAlarm(ctx, alarm, history, knowledge)
	if err != nil {
		if e.Metrics != nil {
			e.Metrics.Inc("ai_analysis_failed_total")
		}
		analysis = model.AIAnalysis{AlarmID: alarm.ID, RiskLevel: alarm.AlarmLevel, CreatedAt: e.Clock.Now().UnixMilli(), Error: err.Error()}
	}
	if err == nil && e.Metrics != nil {
		e.Metrics.Inc("ai_analysis_success_total")
	}
	analysis.TenantID = alarm.TenantID
	analysis.AlarmID = alarm.ID
	if saveErr := e.Repo.SaveAIAnalysis(ctx, analysis); saveErr != nil {
		return analysis, saveErr
	}
	payload := mustJSON(analysis)
	_ = e.Bus.Publish(ctx, model.TopicAlarmAIAnalysis, alarm.ID, payload)
	_ = e.Realtime.Publish(ctx, alarm.MQTTTopic("ai-analysis"), payload, 1, false)
	return analysis, nil
}
func (e *Engine) SetAlarmStatus(ctx context.Context, tenant, alarmID, status, actor string) (model.Alarm, error) {
	a, err := e.Repo.GetAlarm(ctx, tenant, alarmID)
	if err != nil {
		return a, err
	}
	now := e.Clock.Now().UnixMilli()
	switch status {
	case "ACKED":
		if a.Status != "ACTIVE" {
			return a, fmt.Errorf("only active alarms can be acknowledged")
		}
		a.Status = status
		a.AckedAt = now
	case "CLOSED":
		a.Status = status
		a.ClosedAt = now
	case "SUPPRESSED":
		a.Status = status
	default:
		return a, fmt.Errorf("unsupported status %s", status)
	}
	if err := e.Repo.UpdateAlarm(ctx, a); err != nil {
		return a, err
	}
	_ = e.Repo.SaveAudit(ctx, model.AuditLog{ID: id("audit"), TenantID: tenant, Actor: actor, Action: "alarm." + strings.ToLower(status), TargetType: "alarm", TargetID: alarmID, CreatedAt: now})
	payload := mustJSON(a)
	_ = e.Bus.Publish(ctx, model.TopicAlarmConfirmed, a.ID, payload)
	_ = e.Realtime.Publish(ctx, a.MQTTTopic("confirmed"), payload, 1, false)
	return a, nil
}
func tag(m model.StandardMessage, k, fallback string) string {
	if v := m.Tags[k]; v != "" {
		return v
	}
	return fallback
}
func id(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
