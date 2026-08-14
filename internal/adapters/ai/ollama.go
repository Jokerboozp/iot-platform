package aiadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"iot-platform/internal/model"
)

type Ollama struct {
	baseURL, model string
	http           *http.Client
}

func NewOllama(url, model string) *Ollama {
	return &Ollama{strings.TrimRight(url, "/"), model, &http.Client{Timeout: 2 * time.Minute}}
}

type chatRequest struct {
	Model    string              `json:"model"`
	Stream   bool                `json:"stream"`
	Format   any                 `json:"format,omitempty"`
	Messages []map[string]string `json:"messages"`
}
type chatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

func (o *Ollama) call(ctx context.Context, system, user string) (string, error) {
	body, _ := json.Marshal(chatRequest{Model: o.model, Stream: false, Messages: []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}}})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("ollama %s: %s", resp.Status, string(b))
	}
	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Message.Content, nil
}
func (o *Ollama) AnalyzeAlarm(ctx context.Context, a model.Alarm, history []map[string]any, knowledge []string) (model.AIAnalysis, error) {
	input, _ := json.Marshal(map[string]any{"alarm": a, "recentHistory": history, "knowledge": knowledge})
	system := "你是消防物联网运维专家。只能提供分析和人工处置建议，禁止建议自动控制设备。请严格输出 JSON：summary 字符串、possibleReasons 字符串数组、suggestions 字符串数组、riskLevel、confidence(0-1)。"
	content, err := o.call(ctx, system, string(input))
	if err != nil {
		return model.AIAnalysis{}, err
	}
	content = extractJSON(content)
	var out model.AIAnalysis
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return out, fmt.Errorf("decode model json: %w", err)
	}
	out.AlarmID = a.ID
	out.Model = o.model
	out.PromptVersion = "alarm-diagnosis-v1"
	out.CreatedAt = time.Now().UnixMilli()
	return out, nil
}
func (o *Ollama) Chat(ctx context.Context, tenant, question string) (string, error) {
	return o.call(ctx, "你是消防物联网运维助手。回答必须基于提供的受控平台数据；缺少数据时明确说明，不能编造，也不能直接控制设备。租户："+tenant, question)
}
func (o *Ollama) RuleDraft(ctx context.Context, tenant, text string) (model.AlarmRule, error) {
	content, err := o.call(ctx, "将自然语言告警要求转成 JSON 规则草稿。字段必须包含 name,alarmType,level,match,conditions,durationSeconds,recovery；condition 使用 field/operator/value。不要输出 Markdown。租户："+tenant, text)
	if err != nil {
		return model.AlarmRule{}, err
	}
	var rule model.AlarmRule
	if err := json.Unmarshal([]byte(extractJSON(content)), &rule); err != nil {
		return rule, err
	}
	rule.TenantID = tenant
	rule.Enabled = false
	rule.Version = 1
	rule.CreatedAt = time.Now().UnixMilli()
	rule.UpdatedAt = rule.CreatedAt
	return rule, nil
}
func (o *Ollama) Health(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/api/tags", nil)
	resp, err := o.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("ollama %s", resp.Status)
	}
	return nil
}
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

type NoopAI struct{}

func (NoopAI) AnalyzeAlarm(_ context.Context, a model.Alarm, _ []map[string]any, _ []string) (model.AIAnalysis, error) {
	return model.AIAnalysis{AlarmID: a.ID, Summary: "AI 模型未启用，已保留告警供人工研判。", RiskLevel: a.AlarmLevel, Confidence: 0, Model: "disabled", PromptVersion: "fallback-v1", CreatedAt: time.Now().UnixMilli()}, nil
}
func (NoopAI) Chat(context.Context, string, string) (string, error) {
	return "AI 模型未启用。请配置 IOT_OLLAMA_URL 与模型后重试。", nil
}
func (NoopAI) RuleDraft(context.Context, string, string) (model.AlarmRule, error) {
	return model.AlarmRule{}, fmt.Errorf("AI model disabled")
}
func (NoopAI) Health(context.Context) error { return nil }
