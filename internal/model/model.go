package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	TopicRaw             = "iot.raw.message"
	TopicParseFailed     = "iot.parse.failed"
	TopicPropertyReport  = "iot.property.report"
	TopicEventReport     = "iot.event.report"
	TopicDeviceState     = "iot.device.state"
	TopicVideoAlarm      = "iot.video.alarm"
	TopicAlarmRaised     = "iot.alarm.raised"
	TopicAlarmRecovered  = "iot.alarm.recovered"
	TopicAlarmConfirmed  = "iot.alarm.confirmed"
	TopicAlarmAIAnalysis = "iot.alarm.ai-analysis"
	TopicReplayRequest   = "iot.replay.request"
)

type RawMessage struct {
	MessageID     string            `json:"messageId"`
	Source        string            `json:"source"`
	TenantID      string            `json:"tenantId"`
	ProductID     string            `json:"productId"`
	DeviceID      string            `json:"deviceId"`
	DeviceName    string            `json:"deviceName,omitempty"`
	GatewayID     string            `json:"gatewayId,omitempty"`
	Protocol      string            `json:"protocol"`
	Transport     string            `json:"transport"`
	ReceivedAt    int64             `json:"receivedAt"`
	PayloadFormat string            `json:"payloadFormat"`
	Payload       json.RawMessage   `json:"payload"`
	ClientID      string            `json:"clientId,omitempty"`
	RemoteAddress string            `json:"remoteAddress,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	ParserVersion string            `json:"parserVersion,omitempty"`
}

func (m *RawMessage) Normalize(now time.Time) {
	if m.MessageID == "" {
		h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d:%s", m.TenantID, m.DeviceID, now.UnixNano(), m.Payload)))
		m.MessageID = "raw_" + hex.EncodeToString(h[:12])
	}
	if m.Source == "" {
		m.Source = "external-ingest"
	}
	if m.ReceivedAt == 0 {
		m.ReceivedAt = now.UnixMilli()
	}
	if m.PayloadFormat == "" {
		m.PayloadFormat = "json"
	}
}

func (m RawMessage) Validate() error {
	missing := make([]string, 0, 4)
	for name, value := range map[string]string{"messageId": m.MessageID, "tenantId": m.TenantID, "productId": m.ProductID, "deviceId": m.DeviceID} {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	if len(m.Payload) == 0 {
		return errors.New("payload is required")
	}
	return nil
}

func (m RawMessage) PayloadHash() string {
	h := sha256.Sum256(m.Payload)
	return hex.EncodeToString(h[:])
}

type RawArchiveIndex struct {
	MessageID        string `json:"messageId"`
	TenantID         string `json:"tenantId"`
	ProductID        string `json:"productId"`
	DeviceID         string `json:"deviceId"`
	Protocol         string `json:"protocol"`
	PayloadFormat    string `json:"payloadFormat"`
	ObjectBucket     string `json:"objectBucket"`
	ObjectKey        string `json:"objectKey"`
	ObjectOffset     int64  `json:"objectOffset"`
	PayloadHash      string `json:"payloadHash"`
	PayloadSize      int    `json:"payloadSize"`
	ReceivedAt       int64  `json:"receivedAt"`
	ArchivedAt       int64  `json:"archivedAt"`
	PublishedAt      int64  `json:"publishedAt,omitempty"`
	PublishAttempts  int    `json:"publishAttempts,omitempty"`
	LastPublishError string `json:"lastPublishError,omitempty"`
}

type MessageType string

const (
	PropertyReport MessageType = "PROPERTY_REPORT"
	EventReport    MessageType = "EVENT_REPORT"
	StateChange    MessageType = "STATE_CHANGE"
	AlarmReport    MessageType = "ALARM_REPORT"
	CommandReply   MessageType = "COMMAND_REPLY"
	LogReport      MessageType = "LOG_REPORT"
)

type StandardMessage struct {
	MessageID     string            `json:"messageId"`
	RawMessageID  string            `json:"rawMessageId"`
	TenantID      string            `json:"tenantId"`
	ProductID     string            `json:"productId"`
	DeviceID      string            `json:"deviceId"`
	MessageType   MessageType       `json:"messageType"`
	Timestamp     int64             `json:"timestamp"`
	Properties    map[string]any    `json:"properties,omitempty"`
	Event         map[string]any    `json:"event,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
	Raw           map[string]any    `json:"raw,omitempty"`
	Parser        string            `json:"parser"`
	ParserVersion string            `json:"parserVersion"`
}

type DeviceState struct {
	TenantID            string `json:"tenantId"`
	ProductID           string `json:"productId"`
	DeviceID            string `json:"deviceId"`
	ConnectionStatus    string `json:"connectionStatus"`
	DataStatus          string `json:"dataStatus"`
	BusinessStatus      string `json:"businessStatus"`
	LastConnectAt       int64  `json:"lastConnectAt,omitempty"`
	LastDisconnectAt    int64  `json:"lastDisconnectAt,omitempty"`
	LastSeenAt          int64  `json:"lastSeenAt,omitempty"`
	LastMessageID       string `json:"lastMessageId,omitempty"`
	ReportIntervalSec   int64  `json:"reportIntervalSec"`
	OfflineToleranceSec int64  `json:"offlineToleranceSec"`
	OfflineAt           int64  `json:"offlineAt,omitempty"`
	OfflineDetectedAt   int64  `json:"offlineDetectedAt,omitempty"`
	StatusSource        string `json:"statusSource"`
	Reason              string `json:"reason,omitempty"`
}

// Product describes a managed device product and the protocol package used to
// turn its raw payloads into standard platform messages.
type Product struct {
	ID                string         `json:"id"`
	TenantID          string         `json:"tenantId"`
	Name              string         `json:"name"`
	Category          string         `json:"category"`
	ProtocolPackageID string         `json:"protocolPackageId"`
	Transport         string         `json:"transport"`
	PayloadFormat     string         `json:"payloadFormat"`
	Status            string         `json:"status"`
	Description       string         `json:"description,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	CreatedAt         int64          `json:"createdAt"`
	UpdatedAt         int64          `json:"updatedAt"`
}

// ProtocolPackage is a declarative protocol-package release. ParserType points
// at a reviewed parser registered in the Go process; executable code is never
// uploaded through the management API.
type ProtocolPackage struct {
	ID            string         `json:"id"`
	TenantID      string         `json:"tenantId"`
	Name          string         `json:"name"`
	Version       string         `json:"version"`
	Protocol      string         `json:"protocol"`
	Transport     string         `json:"transport"`
	PayloadFormat string         `json:"payloadFormat"`
	ParserType    string         `json:"parserType"`
	Status        string         `json:"status"`
	Description   string         `json:"description,omitempty"`
	Config        map[string]any `json:"config,omitempty"`
	CreatedAt     int64          `json:"createdAt"`
	UpdatedAt     int64          `json:"updatedAt"`
}

// ManagedDevice is the inventory/control-plane record. Runtime connectivity is
// kept separately in DeviceState and joined by the API.
type ManagedDevice struct {
	ID                 string            `json:"id"`
	TenantID           string            `json:"tenantId"`
	ProductID          string            `json:"productId"`
	Name               string            `json:"name"`
	Status             string            `json:"status"`
	DeviceRole         string            `json:"deviceRole"`
	GatewayID          string            `json:"gatewayId,omitempty"`
	RegistrationSource string            `json:"registrationSource,omitempty"`
	AutoRegistered     bool              `json:"autoRegistered,omitempty"`
	AccessKey          string            `json:"accessKey"`
	SecretHash         string            `json:"-"`
	SecretHint         string            `json:"secretHint,omitempty"`
	Description        string            `json:"description,omitempty"`
	Tags               map[string]string `json:"tags,omitempty"`
	CreatedAt          int64             `json:"createdAt"`
	UpdatedAt          int64             `json:"updatedAt"`
}

type DeviceCredential struct {
	AccessKey string `json:"accessKey"`
	Secret    string `json:"secret"`
}

type RuleCondition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

type AlarmRule struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenantId"`
	ProductID       string          `json:"productId,omitempty"`
	Name            string          `json:"name"`
	AlarmType       string          `json:"alarmType"`
	Level           string          `json:"level"`
	Conditions      []RuleCondition `json:"conditions"`
	Match           string          `json:"match"`
	DurationSeconds int64           `json:"durationSeconds"`
	Recovery        []RuleCondition `json:"recovery,omitempty"`
	Expression      string          `json:"expression,omitempty"`
	Enabled         bool            `json:"enabled"`
	Version         int             `json:"version"`
	CreatedAt       int64           `json:"createdAt"`
	UpdatedAt       int64           `json:"updatedAt"`
}

type Alarm struct {
	ID               string         `json:"alarmId"`
	TenantID         string         `json:"tenantId"`
	RuleID           string         `json:"ruleId"`
	DeviceID         string         `json:"deviceId"`
	DeviceName       string         `json:"deviceName,omitempty"`
	AlarmType        string         `json:"alarmType"`
	AlarmLevel       string         `json:"alarmLevel"`
	Status           string         `json:"status"`
	Source           string         `json:"source"`
	CityCode         string         `json:"cityCode"`
	DistrictCode     string         `json:"districtCode"`
	BuildingID       string         `json:"buildingId"`
	DeviceType       string         `json:"deviceType"`
	AreaID           string         `json:"areaId,omitempty"`
	FirstTriggeredAt int64          `json:"firstTriggeredAt"`
	LastTriggeredAt  int64          `json:"lastTriggeredAt"`
	TriggerCount     int            `json:"triggerCount"`
	RecoveredAt      int64          `json:"recoveredAt,omitempty"`
	AckedAt          int64          `json:"ackedAt,omitempty"`
	ClosedAt         int64          `json:"closedAt,omitempty"`
	Confidence       float64        `json:"confidence,omitempty"`
	MultiSource      bool           `json:"multiSource"`
	Details          map[string]any `json:"details,omitempty"`
}

func (a Alarm) MQTTTopic(eventType string) string {
	clean := func(v string) string {
		v = strings.Trim(strings.ReplaceAll(v, "/", "_"), " ")
		if v == "" {
			return "unknown"
		}
		return v
	}
	return fmt.Sprintf("/iot/alarm/%s/%s/%s/%s/%s/%s", clean(eventType), clean(a.CityCode), clean(a.DistrictCode), clean(a.BuildingID), clean(a.DeviceType), clean(a.DeviceID))
}

type VideoAlarmEvent struct {
	EventID      string         `json:"eventId"`
	Source       string         `json:"source"`
	TenantID     string         `json:"tenantId"`
	ProjectID    string         `json:"projectId"`
	CameraID     string         `json:"cameraId"`
	CameraName   string         `json:"cameraName"`
	AreaID       string         `json:"areaId"`
	AlarmType    string         `json:"alarmType"`
	AlarmName    string         `json:"alarmName"`
	AlarmLevel   string         `json:"alarmLevel"`
	Confidence   float64        `json:"confidence"`
	EventTime    int64          `json:"eventTime"`
	ReceivedAt   int64          `json:"receivedAt"`
	SnapshotURL  string         `json:"snapshotUrl,omitempty"`
	VideoClipURL string         `json:"videoClipUrl,omitempty"`
	CityCode     string         `json:"cityCode"`
	DistrictCode string         `json:"districtCode"`
	BuildingID   string         `json:"buildingId"`
	Location     map[string]any `json:"location,omitempty"`
	Raw          map[string]any `json:"raw,omitempty"`
}

type AIAnalysis struct {
	TenantID        string   `json:"tenantId,omitempty"`
	AlarmID         string   `json:"alarmId"`
	Summary         string   `json:"summary"`
	PossibleReasons []string `json:"possibleReasons"`
	Suggestions     []string `json:"suggestions"`
	RiskLevel       string   `json:"riskLevel"`
	Confidence      float64  `json:"confidence"`
	Model           string   `json:"model"`
	PromptVersion   string   `json:"promptVersion"`
	CreatedAt       int64    `json:"createdAt"`
	Error           string   `json:"error,omitempty"`
}

type KnowledgeDoc struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenantId"`
	ProductID    string         `json:"productId,omitempty"`
	Category     string         `json:"category,omitempty"`
	Tags         []string       `json:"tags,omitempty"`
	ObjectBucket string         `json:"objectBucket"`
	ObjectKey    string         `json:"objectKey"`
	Filename     string         `json:"filename"`
	Status       string         `json:"status"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	CreatedAt    int64          `json:"createdAt"`
}

// WorkflowKnowledgeBinding constrains knowledge retrieval for one tenant and
// one Harness workflow. The plugin manifest defines what the workflow can do;
// this record defines which tenant knowledge it may use.
type WorkflowKnowledgeBinding struct {
	TenantID      string   `json:"tenantId"`
	WorkflowID    string   `json:"workflowId"`
	ProductIDs    []string `json:"productIds,omitempty"`
	Categories    []string `json:"categories,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	RetrievalMode string   `json:"retrievalMode"`
	TopK          int      `json:"topK"`
	MinScore      float64  `json:"minScore"`
	NoMatchPolicy string   `json:"noMatchPolicy"`
	UpdatedAt     int64    `json:"updatedAt"`
}

type ReplayRequest struct {
	ID            string         `json:"id"`
	TenantID      string         `json:"tenantId"`
	ProductID     string         `json:"productId,omitempty"`
	DeviceID      string         `json:"deviceId,omitempty"`
	Start         int64          `json:"start"`
	End           int64          `json:"end"`
	ParserVersion string         `json:"parserVersion,omitempty"`
	Mode          string         `json:"mode"`
	RatePerSecond int            `json:"ratePerSecond"`
	Status        string         `json:"status"`
	Processed     int            `json:"processed"`
	Failed        int            `json:"failed"`
	CreatedBy     string         `json:"createdBy"`
	CreatedAt     int64          `json:"createdAt"`
	CompletedAt   int64          `json:"completedAt,omitempty"`
	DiffSummary   map[string]int `json:"diffSummary,omitempty"`
	Diffs         []ReplayDiff   `json:"diffs,omitempty"`
}

type ReplayDiff struct {
	RawMessageID string           `json:"rawMessageId"`
	Status       string           `json:"status"`
	Previous     *StandardMessage `json:"previous,omitempty"`
	Current      *StandardMessage `json:"current,omitempty"`
	Error        string           `json:"error,omitempty"`
}

type VideoCameraMapping struct {
	TenantID         string   `json:"tenantId"`
	CameraID         string   `json:"cameraId"`
	CameraName       string   `json:"cameraName"`
	ProjectID        string   `json:"projectId,omitempty"`
	CityCode         string   `json:"cityCode,omitempty"`
	DistrictCode     string   `json:"districtCode,omitempty"`
	Building         string   `json:"building,omitempty"`
	Floor            string   `json:"floor,omitempty"`
	AreaID           string   `json:"areaId"`
	RelatedDeviceIDs []string `json:"relatedDeviceIds,omitempty"`
	VideoPlatformID  string   `json:"videoPlatformId,omitempty"`
	StreamURL        string   `json:"streamUrl,omitempty"`
	StreamType       string   `json:"streamType,omitempty"`
	StreamConfigured bool     `json:"streamConfigured,omitempty"`
	PreviewEligible  bool     `json:"previewEligible,omitempty"`
	Enabled          bool     `json:"enabled"`
	UpdatedAt        int64    `json:"updatedAt"`
}

type AIToolCallLog struct {
	ID        string         `json:"id"`
	TenantID  string         `json:"tenantId"`
	Actor     string         `json:"actor"`
	Tool      string         `json:"tool"`
	Input     map[string]any `json:"input,omitempty"`
	Output    any            `json:"output,omitempty"`
	Success   bool           `json:"success"`
	Error     string         `json:"error,omitempty"`
	CreatedAt int64          `json:"createdAt"`
}

type ExternalAuth struct {
	Username      string `json:"username"`
	TenantID      string `json:"tenantId"`
	Role          string `json:"role"`
	UpstreamToken string `json:"-"`
}
type CatalogSyncResult struct {
	Products int      `json:"products"`
	Devices  int      `json:"devices"`
	SyncedAt int64    `json:"syncedAt"`
	Errors   []string `json:"errors,omitempty"`
}

type AuditLog struct {
	ID         string         `json:"id"`
	TenantID   string         `json:"tenantId"`
	Actor      string         `json:"actor"`
	Action     string         `json:"action"`
	TargetType string         `json:"targetType"`
	TargetID   string         `json:"targetId"`
	Details    map[string]any `json:"details,omitempty"`
	CreatedAt  int64          `json:"createdAt"`
}
