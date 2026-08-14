package core

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"iot-platform/internal/model"
	"iot-platform/internal/ports"
)

var deviceIDPattern = regexp.MustCompile(`(?i)\b(device[_-][a-z0-9_-]+)\b`)

// OpsChat gathers data only through controlled repository/knowledge ports. The model never receives SQL access.
func (e *Engine) OpsChat(ctx context.Context, tenantID, question string) (string, error) {
	contextData := map[string]any{}
	alarms, err := e.Repo.ListAlarms(ctx, ports.AlarmFilter{TenantID: tenantID, Status: "ACTIVE", Start: time.Now().Add(-24 * time.Hour).UnixMilli(), Limit: 50})
	if err == nil {
		contextData["activeAlarmsLast24h"] = alarms
	}
	if match := deviceIDPattern.FindStringSubmatch(question); len(match) > 1 {
		if state, stateErr := e.Repo.GetDeviceState(ctx, tenantID, match[1]); stateErr == nil {
			contextData["deviceLatest"] = state
		}
		if history, historyErr := e.Repo.PropertyHistory(ctx, tenantID, match[1], "temperature", time.Now().Add(-24*time.Hour).UnixMilli(), time.Now().UnixMilli(), 500); historyErr == nil {
			contextData["temperatureHistory"] = history
		}
	}
	if e.KB != nil {
		if docs, kbErr := e.KB.Search(ctx, tenantID, question, 5); kbErr == nil {
			contextData["knowledge"] = docs
		}
	}
	b, _ := json.Marshal(contextData)
	answer, err := e.AI.Chat(ctx, tenantID, "用户问题："+question+"\n受控平台工具返回的数据："+string(b)+"\n仅依据这些数据作答；数据不足时明确说明。")
	_ = e.Repo.SaveAudit(ctx, model.AuditLog{ID: id("audit"), TenantID: tenantID, Actor: "ai-ops-chat", Action: "ai.chat", TargetType: "conversation", TargetID: id("chat"), Details: map[string]any{"question": question, "success": err == nil}, CreatedAt: e.Clock.Now().UnixMilli()})
	return answer, err
}

func (e *Engine) GenerateReport(ctx context.Context, tenantID, period string, start, end int64) (string, error) {
	if start <= 0 || end <= start {
		return "", fmt.Errorf("valid start/end are required")
	}
	alarms, err := e.Repo.ListAlarms(ctx, ports.AlarmFilter{TenantID: tenantID, Start: start, End: end, Limit: 5000})
	if err != nil {
		return "", err
	}
	states, _ := e.Repo.ListDeviceStates(ctx, tenantID)
	payload, _ := json.Marshal(map[string]any{"period": period, "start": start, "end": end, "alarms": alarms, "deviceStates": states})
	report, err := e.AI.Chat(ctx, tenantID, "请根据受控平台数据生成消防物联网"+period+"报告，包含告警概况、高等级风险、设备离线情况、趋势、处置建议和数据局限。数据："+string(payload))
	_ = e.Repo.SaveAudit(ctx, model.AuditLog{ID: id("audit"), TenantID: tenantID, Actor: "ai-report-generator", Action: "ai.report", TargetType: "report", TargetID: id("report"), Details: map[string]any{"period": period, "start": start, "end": end, "success": err == nil}, CreatedAt: e.Clock.Now().UnixMilli()})
	return report, err
}
