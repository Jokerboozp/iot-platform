package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr                    string
	CORSAllowedOrigins          []string
	DataDir                     string
	JWTSecret                   string
	AdminUser                   string
	AdminPassword               string
	AdminTenants                []string
	PostgresDSN                 string
	RedisAddr                   string
	RedisPassword               string
	ClickHouseURL               string
	RawHighFrequencyIntervalSec int64
	MinIOEndpoint               string
	MinIOAccessKey              string
	MinIOSecretKey              string
	MinIOUseTLS                 bool
	KafkaBrokers                []string
	MQTTBroker                  string
	MQTTUsername                string
	MQTTPassword                string
	MQTTWebSocketURL            string
	OllamaURL                   string
	OllamaModel                 string
	AIProvider                  string
	AIBaseURL                   string
	AIModel                     string
	AIAPIKey                    string
	AIHarnessURL                string
	AIHarnessToken              string
	AIHarnessMCPURL             string
	AIHarnessModel              string
	AIHarnessTimeout            time.Duration
	AITestOrigins               []string
	AITestOllamaURL             string
	WeaviateURL                 string
	BackupURL                   string
	BackupToken                 string
	VideoSecrets                map[string]string
	VideoPlatformTenants        map[string]string
	VideoMediaHosts             []string
	VideoPreviewOrigins         []string
	VideoZLMAPIURL              string
	VideoZLMPlaybackURL         string
	VideoZLMSecret              string
	VideoZLMVhost               string
	VideoZLMApp                 string
	DahuaVideoSDKURL            string
	DahuaVideoSDKToken          string
	HikvisionVideoAPIURL        string
	HikvisionAppKey             string
	HikvisionAppSecret          string
	ThingsPanelURL              string
	ThingsPanelUser             string
	ThingsPanelPassword         string
	ThingsPanelSync             time.Duration
	OfflineScan                 time.Duration
	DevMode                     bool
}

func Load() Config {
	aiProvider := strings.ToLower(strings.TrimSpace(os.Getenv("IOT_AI_PROVIDER")))
	deepSeekAPIKey := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	if aiProvider == "" && deepSeekAPIKey != "" {
		aiProvider = "deepseek"
	}
	aiAPIKey := strings.TrimSpace(os.Getenv("IOT_AI_API_KEY"))
	if aiAPIKey == "" && aiProvider == "deepseek" {
		aiAPIKey = deepSeekAPIKey
	}
	return Config{
		HTTPAddr:                    get("IOT_HTTP_ADDR", ":8080"),
		CORSAllowedOrigins:          split(os.Getenv("IOT_CORS_ALLOWED_ORIGINS")),
		DataDir:                     get("IOT_DATA_DIR", "./data"),
		JWTSecret:                   get("IOT_JWT_SECRET", "change-me-in-production"),
		AdminUser:                   get("IOT_ADMIN_USER", "admin"),
		AdminPassword:               get("IOT_ADMIN_PASSWORD", "admin123"),
		AdminTenants:                split(get("IOT_ADMIN_TENANTS", "tenant_001")),
		PostgresDSN:                 os.Getenv("IOT_POSTGRES_DSN"),
		RedisAddr:                   os.Getenv("IOT_REDIS_ADDR"),
		RedisPassword:               os.Getenv("IOT_REDIS_PASSWORD"),
		ClickHouseURL:               os.Getenv("IOT_CLICKHOUSE_URL"),
		RawHighFrequencyIntervalSec: int64Value("IOT_RAW_HIGH_FREQUENCY_INTERVAL_SEC", 60),
		MinIOEndpoint:               os.Getenv("IOT_MINIO_ENDPOINT"),
		MinIOAccessKey:              os.Getenv("IOT_MINIO_ACCESS_KEY"),
		MinIOSecretKey:              os.Getenv("IOT_MINIO_SECRET_KEY"),
		MinIOUseTLS:                 boolValue("IOT_MINIO_USE_TLS", false),
		KafkaBrokers:                split(os.Getenv("IOT_KAFKA_BROKERS")),
		MQTTBroker:                  os.Getenv("IOT_MQTT_BROKER"),
		MQTTUsername:                os.Getenv("IOT_MQTT_USERNAME"),
		MQTTPassword:                os.Getenv("IOT_MQTT_PASSWORD"),
		MQTTWebSocketURL:            os.Getenv("IOT_MQTT_WEBSOCKET_PUBLIC_URL"),
		OllamaURL:                   get("IOT_OLLAMA_URL", "http://localhost:11434"),
		OllamaModel:                 get("IOT_OLLAMA_MODEL", "qwen3:8b"),
		AIProvider:                  aiProvider,
		AIBaseURL:                   strings.TrimRight(os.Getenv("IOT_AI_BASE_URL"), "/"),
		AIModel:                     strings.TrimSpace(os.Getenv("IOT_AI_MODEL")),
		AIAPIKey:                    aiAPIKey,
		AIHarnessURL:                strings.TrimRight(strings.TrimSpace(os.Getenv("IOT_AI_HARNESS_URL")), "/"),
		AIHarnessToken:              strings.TrimSpace(os.Getenv("IOT_AI_HARNESS_TOKEN")),
		AIHarnessMCPURL:             strings.TrimSpace(os.Getenv("IOT_AI_HARNESS_MCP_URL")),
		AIHarnessModel:              strings.TrimSpace(os.Getenv("IOT_AI_HARNESS_MODEL")),
		AIHarnessTimeout:            duration("IOT_AI_HARNESS_TIMEOUT", 90*time.Second),
		AITestOrigins:               split(get("IOT_AI_PROVIDER_TEST_ALLOWED_ORIGINS", "https://api.deepseek.com,http://localhost:11434,http://127.0.0.1:11434,http://[::1]:11434,http://ollama:11434")),
		AITestOllamaURL:             get("IOT_AI_OLLAMA_URL", get("IOT_OLLAMA_URL", "http://localhost:11434")),
		WeaviateURL:                 os.Getenv("IOT_WEAVIATE_URL"),
		BackupURL:                   strings.TrimRight(strings.TrimSpace(os.Getenv("IOT_BACKUP_URL")), "/"),
		BackupToken:                 strings.TrimSpace(os.Getenv("IOT_BACKUP_ADMIN_TOKEN")),
		VideoSecrets:                parsePairs(os.Getenv("IOT_VIDEO_PLATFORM_SECRETS")),
		VideoPlatformTenants:        parsePairs(os.Getenv("IOT_VIDEO_PLATFORM_TENANTS")),
		VideoMediaHosts:             split(os.Getenv("IOT_VIDEO_MEDIA_ALLOWED_HOSTS")),
		VideoPreviewOrigins:         split(os.Getenv("IOT_VIDEO_PREVIEW_ALLOWED_ORIGINS")),
		VideoZLMAPIURL:              strings.TrimRight(strings.TrimSpace(os.Getenv("IOT_VIDEO_ZLM_API_URL")), "/"),
		VideoZLMPlaybackURL:         strings.TrimRight(strings.TrimSpace(os.Getenv("IOT_VIDEO_ZLM_PLAYBACK_BASE_URL")), "/"),
		VideoZLMSecret:              strings.TrimSpace(os.Getenv("IOT_VIDEO_ZLM_SECRET")),
		VideoZLMVhost:               get("IOT_VIDEO_ZLM_VHOST", "__defaultVhost__"),
		VideoZLMApp:                 get("IOT_VIDEO_ZLM_APP", "iot"),
		DahuaVideoSDKURL:            strings.TrimRight(strings.TrimSpace(os.Getenv("IOT_VIDEO_DAHUA_SDK_URL")), "/"),
		DahuaVideoSDKToken:          strings.TrimSpace(os.Getenv("IOT_VIDEO_DAHUA_SDK_TOKEN")),
		HikvisionVideoAPIURL:        strings.TrimRight(strings.TrimSpace(os.Getenv("IOT_VIDEO_HIKVISION_API_URL")), "/"),
		HikvisionAppKey:             strings.TrimSpace(os.Getenv("IOT_VIDEO_HIKVISION_APP_KEY")),
		HikvisionAppSecret:          strings.TrimSpace(os.Getenv("IOT_VIDEO_HIKVISION_APP_SECRET")),
		ThingsPanelURL:              strings.TrimRight(os.Getenv("IOT_THINGSPANEL_URL"), "/"),
		ThingsPanelUser:             os.Getenv("IOT_THINGSPANEL_USER"),
		ThingsPanelPassword:         os.Getenv("IOT_THINGSPANEL_PASSWORD"),
		ThingsPanelSync:             duration("IOT_THINGSPANEL_SYNC_INTERVAL", 5*time.Minute),
		OfflineScan:                 duration("IOT_OFFLINE_SCAN_INTERVAL", 30*time.Second),
		DevMode:                     boolValue("IOT_DEV_MODE", true),
	}
}

func get(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
func split(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	values := strings.Split(v, ",")
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}
func boolValue(name string, fallback bool) bool {
	v, err := strconv.ParseBool(os.Getenv(name))
	if err != nil {
		return fallback
	}
	return v
}
func int64Value(name string, fallback int64) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(name)), 10, 64)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
func duration(name string, fallback time.Duration) time.Duration {
	v, err := time.ParseDuration(os.Getenv(name))
	if err != nil {
		return fallback
	}
	return v
}
func parsePairs(v string) map[string]string {
	out := map[string]string{}
	for _, pair := range strings.Split(v, ",") {
		p := strings.SplitN(pair, ":", 2)
		if len(p) == 2 {
			out[strings.TrimSpace(p[0])] = strings.TrimSpace(p[1])
		}
	}
	return out
}
