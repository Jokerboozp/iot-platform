package aiadapter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"iot-platform/internal/ports"
)

const (
	maxHarnessResponseBytes = 1 << 20
	maxHarnessEventBytes    = 64 << 10
)

type HarnessClient struct {
	baseURL string
	token   string
	mcpURL  string
	model   string
	client  *http.Client
}

func NewHarness(baseURL, token, mcpURL, model string, timeout time.Duration) (*HarnessClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if err := validateHTTPURL(baseURL); err != nil {
		return nil, fmt.Errorf("invalid harness URL: %w", err)
	}
	token = strings.TrimSpace(token)
	if len(token) < 32 || len(token) > 512 {
		return nil, errors.New("harness service token must contain 32 to 512 characters")
	}
	if strings.TrimSpace(mcpURL) == "" {
		return nil, errors.New("harness MCP URL is required")
	}
	if err := validateHTTPURL(mcpURL); err != nil {
		return nil, fmt.Errorf("invalid harness MCP URL: %w", err)
	}
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	return &HarnessClient{
		baseURL: baseURL,
		token:   token,
		mcpURL:  strings.TrimSpace(mcpURL),
		model:   strings.TrimSpace(model),
		client:  &http.Client{Timeout: timeout},
	}, nil
}

func validateHTTPURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("an absolute http(s) URL without userinfo is required")
	}
	return nil
}

func (h *HarnessClient) ListWorkflows(ctx context.Context) ([]ports.AIWorkflowPlugin, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.baseURL+"/v1/plugins", nil)
	if err != nil {
		return nil, err
	}
	h.authorize(req)
	res, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list harness workflows: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return nil, fmt.Errorf("list harness workflows: status %d", res.StatusCode)
	}
	var envelope struct {
		Items   []ports.AIWorkflowPlugin `json:"items"`
		Plugins []ports.AIWorkflowPlugin `json:"plugins"`
	}
	limited := io.LimitReader(res.Body, maxHarnessResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read harness workflows: %w", err)
	}
	if len(body) > maxHarnessResponseBytes {
		return nil, errors.New("harness workflow response exceeds 1 MiB")
	}
	var items []ports.AIWorkflowPlugin
	if err = json.Unmarshal(body, &items); err != nil {
		if err = json.Unmarshal(body, &envelope); err != nil {
			return nil, fmt.Errorf("decode harness workflows: %w", err)
		}
		items = envelope.Items
		if items == nil {
			items = envelope.Plugins
		}
	}
	if items == nil {
		items = []ports.AIWorkflowPlugin{}
	}
	return items, nil
}

func (h *HarnessClient) SaveWorkflow(ctx context.Context, manifest ports.AIWorkflowManifest) (ports.AIWorkflowPlugin, error) {
	var plugin ports.AIWorkflowPlugin
	payload, err := json.Marshal(manifest)
	if err != nil {
		return plugin, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.baseURL+"/v1/plugins", bytes.NewReader(payload))
	if err != nil {
		return plugin, err
	}
	req.Header.Set("Content-Type", "application/json")
	h.authorizeService(req)
	res, err := h.client.Do(req)
	if err != nil {
		return plugin, fmt.Errorf("save harness workflow: %w", err)
	}
	defer res.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(res.Body, maxHarnessResponseBytes+1))
	if readErr != nil {
		return plugin, readErr
	}
	if len(body) > maxHarnessResponseBytes {
		return plugin, errors.New("harness workflow response exceeds 1 MiB")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return plugin, fmt.Errorf("save harness workflow: status %d", res.StatusCode)
	}
	if err = json.Unmarshal(body, &plugin); err != nil {
		return plugin, fmt.Errorf("decode saved harness workflow: %w", err)
	}
	return plugin, nil
}

func (h *HarnessClient) StreamChat(ctx context.Context, in ports.AIWorkflowRequest, emit func(ports.AIWorkflowEvent) error) (ports.AIWorkflowResult, error) {
	if strings.TrimSpace(in.RunID) == "" || strings.TrimSpace(in.Question) == "" || strings.TrimSpace(in.MCPToken) == "" {
		return ports.AIWorkflowResult{}, errors.New("runId, question and MCP token are required")
	}
	if in.WorkflowID == "" {
		in.WorkflowID = "ops-assistant"
	}
	if in.ConversationID == "" {
		in.ConversationID = in.RunID
	}
	if in.Model == "" {
		in.Model = h.model
	}
	if in.MCPURL == "" {
		in.MCPURL = h.mcpURL
	}
	if in.MaxTokens <= 0 {
		in.MaxTokens = 2048
	}
	if in.MaxTokens > 8192 {
		in.MaxTokens = 8192
	}
	// Keep the short-lived MCP credential out of the JSON payload. The sidecar
	// receives it only as Authorization and can forward it to the MCP bridge.
	payload, err := json.Marshal(struct {
		RunID          string `json:"runId"`
		ConversationID string `json:"conversationId"`
		WorkflowID     string `json:"workflowId"`
		Question       string `json:"question"`
		MCPURL         string `json:"mcpUrl"`
		Model          string `json:"model"`
		MaxTokens      int    `json:"maxTokens"`
	}{in.RunID, in.ConversationID, in.WorkflowID, in.Question, in.MCPURL, in.Model, in.MaxTokens})
	if err != nil {
		return ports.AIWorkflowResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.baseURL+"/v1/chat/stream", bytes.NewReader(payload))
	if err != nil {
		return ports.AIWorkflowResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson")
	req.Header.Set("Authorization", "Bearer "+in.MCPToken)
	h.authorizeService(req)
	res, err := h.client.Do(req)
	if err != nil {
		return ports.AIWorkflowResult{}, fmt.Errorf("run harness workflow: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return ports.AIWorkflowResult{}, fmt.Errorf("run harness workflow: status %d", res.StatusCode)
	}

	result := ports.AIWorkflowResult{RunID: in.RunID, WorkflowID: in.WorkflowID, Model: in.Model}
	var streamed strings.Builder
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 4096), maxHarnessEventBytes)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var event ports.AIWorkflowEvent
		if err = json.Unmarshal(line, &event); err != nil {
			return result, fmt.Errorf("decode harness event: %w", err)
		}
		if !AllowedWorkflowEvent(event.Type) {
			return result, fmt.Errorf("unsupported harness event type %q", event.Type)
		}
		if event.RunID == "" {
			event.RunID = in.RunID
		}
		if event.WorkflowID == "" {
			event.WorkflowID = in.WorkflowID
		}
		if event.Model == "" {
			event.Model = in.Model
		}
		if event.Type == "text.delta" {
			if event.Delta != "" {
				streamed.WriteString(event.Delta)
			} else {
				streamed.WriteString(event.Text)
			}
		}
		if event.Answer != "" && event.Type == "run.completed" {
			result.Answer = event.Answer
		}
		if emit != nil {
			if err = emit(event); err != nil {
				return result, err
			}
		}
		if event.Type == "run.failed" {
			return result, errors.New("harness workflow failed")
		}
	}
	if err = scanner.Err(); err != nil {
		return result, fmt.Errorf("read harness event stream: %w", err)
	}
	if result.Answer == "" {
		result.Answer = streamed.String()
	}
	return result, nil
}

func (h *HarnessClient) Health(ctx context.Context) error {
	_, err := h.ListWorkflows(ctx)
	return err
}

func (h *HarnessClient) authorize(req *http.Request) {
	if h.token != "" {
		req.Header.Set("X-IOT-Harness-Token", h.token)
	}
}

func (h *HarnessClient) authorizeService(req *http.Request) { h.authorize(req) }

func AllowedWorkflowEvent(eventType string) bool {
	switch eventType {
	case "run.started", "text.delta", "tool.started", "tool.completed", "run.completed", "run.failed":
		return true
	default:
		return false
	}
}

var _ ports.AIWorkflowRuntime = (*HarnessClient)(nil)
