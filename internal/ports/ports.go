package ports

import (
	"context"
	"io"
	"time"

	"iot-platform/internal/model"
)

type RawFilter struct {
	TenantID, ProductID, DeviceID string
	Start, End                    int64
	Limit, Offset                 int
}

type AlarmFilter struct {
	TenantID, DeviceID, Status, Level, Source string
	Start, End                                int64
	Limit, Offset                             int
}

type Repository interface {
	SaveProduct(context.Context, model.Product) error
	GetProduct(context.Context, string, string) (model.Product, error)
	ListProducts(context.Context, string) ([]model.Product, error)
	SaveProtocolPackage(context.Context, model.ProtocolPackage) error
	GetProtocolPackage(context.Context, string, string) (model.ProtocolPackage, error)
	ListProtocolPackages(context.Context, string) ([]model.ProtocolPackage, error)
	SaveManagedDevice(context.Context, model.ManagedDevice) error
	GetManagedDevice(context.Context, string, string) (model.ManagedDevice, error)
	GetManagedDeviceByAccessKey(context.Context, string) (model.ManagedDevice, error)
	ListManagedDevices(context.Context, string) ([]model.ManagedDevice, error)
	SaveRawIndex(context.Context, model.RawArchiveIndex) (bool, error)
	MarkRawPublished(context.Context, string, string, int64, string) error
	ListPendingRawIndexes(context.Context, int) ([]model.RawArchiveIndex, error)
	GetRawIndex(context.Context, string, string) (model.RawArchiveIndex, error)
	ListRawIndexes(context.Context, RawFilter) ([]model.RawArchiveIndex, error)
	SaveStandardMessage(context.Context, model.StandardMessage) error
	GetStandardMessageByRaw(context.Context, string, string) (model.StandardMessage, error)
	GetLatestMessage(context.Context, string, string) (model.StandardMessage, error)
	PropertyHistory(context.Context, string, string, string, int64, int64, int) ([]map[string]any, error)
	UpsertDeviceState(context.Context, model.DeviceState) error
	GetDeviceState(context.Context, string, string) (model.DeviceState, error)
	ListDeviceStates(context.Context, string) ([]model.DeviceState, error)
	SaveDeviceStateEvent(context.Context, model.DeviceState) error
	SaveRule(context.Context, model.AlarmRule) error
	ListRules(context.Context, string) ([]model.AlarmRule, error)
	DeleteRule(context.Context, string, string) error
	UpsertAlarm(context.Context, model.Alarm) (model.Alarm, bool, error)
	GetAlarm(context.Context, string, string) (model.Alarm, error)
	ListAlarms(context.Context, AlarmFilter) ([]model.Alarm, error)
	UpdateAlarm(context.Context, model.Alarm) error
	SaveVideoEvent(context.Context, model.VideoAlarmEvent) (bool, error)
	UpdateVideoEvent(context.Context, model.VideoAlarmEvent) error
	ListPendingVideoEvents(context.Context, int) ([]model.VideoAlarmEvent, error)
	SaveVideoCameraMapping(context.Context, model.VideoCameraMapping) error
	GetVideoCameraMapping(context.Context, string, string) (model.VideoCameraMapping, error)
	ListVideoCameraMappings(context.Context, string) ([]model.VideoCameraMapping, error)
	SaveAIAnalysis(context.Context, model.AIAnalysis) error
	GetAIAnalysis(context.Context, string, string) (model.AIAnalysis, error)
	SaveKnowledgeDoc(context.Context, model.KnowledgeDoc) error
	SaveReplay(context.Context, model.ReplayRequest) error
	UpdateReplay(context.Context, model.ReplayRequest) error
	GetReplay(context.Context, string) (model.ReplayRequest, error)
	SaveAudit(context.Context, model.AuditLog) error
	SaveAIToolCall(context.Context, model.AIToolCallLog) error
	Health(context.Context) error
	Close() error
}

type Archive interface {
	PutRaw(context.Context, model.RawMessage) (model.RawArchiveIndex, error)
	GetRaw(context.Context, model.RawArchiveIndex) (model.RawMessage, error)
	PutObject(context.Context, string, string, io.Reader, int64, string) (string, error)
	GetObject(context.Context, string, string) (io.ReadCloser, error)
	Health(context.Context) error
}

type Handler func(context.Context, []byte) error

type EventBus interface {
	Publish(context.Context, string, string, []byte) error
	Subscribe(context.Context, string, string, Handler) error
	Health(context.Context) error
	Close() error
}

type RealtimePublisher interface {
	Publish(context.Context, string, []byte, byte, bool) error
	Health(context.Context) error
	Close() error
}

type AIClient interface {
	AnalyzeAlarm(context.Context, model.Alarm, []map[string]any, []string) (model.AIAnalysis, error)
	Chat(context.Context, string, string) (string, error)
	RuleDraft(context.Context, string, string) (model.AlarmRule, error)
	Health(context.Context) error
}

type AIPluginConfig struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"baseUrl,omitempty"`
	Model    string `json:"model,omitempty"`
	APIKey   string `json:"apiKey,omitempty"`
}

type AIPluginInfo struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	DefaultBaseURL string   `json:"defaultBaseUrl,omitempty"`
	DefaultModel   string   `json:"defaultModel,omitempty"`
	Model          string   `json:"model,omitempty"`
	RequiresAPIKey bool     `json:"requiresApiKey"`
	Enabled        bool     `json:"enabled"`
	Capabilities   []string `json:"capabilities"`
}

type AIInspectable interface {
	ProviderInfo() AIPluginInfo
}

type AIPluginRegistry interface {
	List() []AIPluginInfo
	Create(AIPluginConfig) (AIClient, error)
}

// AIWorkflowPlugin describes a business workflow exposed by an external AI
// runtime. Provider plugins and workflow plugins deliberately use separate
// contracts: providers generate text, workflows may orchestrate read-only MCP
// tools and stream progress events.
type AIWorkflowPlugin struct {
	SchemaVersion int      `json:"schemaVersion,omitempty"`
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	Version       string   `json:"version,omitempty"`
	DefaultModel  string   `json:"defaultModel,omitempty"`
	MaxTokens     int      `json:"maxTokens,omitempty"`
	Enabled       bool     `json:"enabled"`
	Capabilities  []string `json:"capabilities,omitempty"`
}

type AIWorkflowRequest struct {
	RunID          string `json:"runId"`
	ConversationID string `json:"conversationId"`
	WorkflowID     string `json:"workflowId"`
	Question       string `json:"question"`
	MCPURL         string `json:"mcpUrl"`
	Model          string `json:"model,omitempty"`
	MaxTokens      int    `json:"maxTokens"`
	MCPToken       string `json:"-"`
}

type AIWorkflowEvent struct {
	Type       string         `json:"type"`
	RunID      string         `json:"runId,omitempty"`
	WorkflowID string         `json:"workflowId,omitempty"`
	Model      string         `json:"model,omitempty"`
	Text       string         `json:"text,omitempty"`
	Delta      string         `json:"delta,omitempty"`
	Answer     string         `json:"answer,omitempty"`
	Message    string         `json:"message,omitempty"`
	Tool       string         `json:"tool,omitempty"`
	CallID     string         `json:"callId,omitempty"`
	Status     string         `json:"status,omitempty"`
	Success    *bool          `json:"success,omitempty"`
	Code       string         `json:"code,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
}

type AIWorkflowResult struct {
	RunID      string `json:"runId"`
	WorkflowID string `json:"workflowId,omitempty"`
	Model      string `json:"model,omitempty"`
	Answer     string `json:"answer"`
}

type AIWorkflowRuntime interface {
	ListWorkflows(context.Context) ([]AIWorkflowPlugin, error)
	StreamChat(context.Context, AIWorkflowRequest, func(AIWorkflowEvent) error) (AIWorkflowResult, error)
	Health(context.Context) error
}

type KnowledgeBase interface {
	Index(context.Context, string, string, string, []byte) error
	Search(context.Context, string, string, int) ([]string, error)
	Health(context.Context) error
}

type PlatformCatalog interface {
	Authenticate(context.Context, string, string) (model.ExternalAuth, error)
	Sync(context.Context, string) (model.CatalogSyncResult, error)
	Health(context.Context) error
}

type Clock interface{ Now() time.Time }
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }
