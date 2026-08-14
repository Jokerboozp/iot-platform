package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr            string
	CORSAllowedOrigins  []string
	DataDir             string
	JWTSecret           string
	AdminUser           string
	AdminPassword       string
	PostgresDSN         string
	RedisAddr           string
	RedisPassword       string
	ClickHouseURL       string
	MinIOEndpoint       string
	MinIOAccessKey      string
	MinIOSecretKey      string
	MinIOUseTLS         bool
	KafkaBrokers        []string
	MQTTBroker          string
	MQTTUsername        string
	MQTTPassword        string
	MQTTWebSocketURL    string
	OllamaURL           string
	OllamaModel         string
	WeaviateURL         string
	VideoSecrets        map[string]string
	VideoMediaHosts     []string
	ThingsPanelURL      string
	ThingsPanelUser     string
	ThingsPanelPassword string
	ThingsPanelSync     time.Duration
	OfflineScan         time.Duration
	DevMode             bool
}

func Load() Config {
	return Config{
		HTTPAddr:            get("IOT_HTTP_ADDR", ":8080"),
		CORSAllowedOrigins:  split(os.Getenv("IOT_CORS_ALLOWED_ORIGINS")),
		DataDir:             get("IOT_DATA_DIR", "./data"),
		JWTSecret:           get("IOT_JWT_SECRET", "change-me-in-production"),
		AdminUser:           get("IOT_ADMIN_USER", "admin"),
		AdminPassword:       get("IOT_ADMIN_PASSWORD", "admin123"),
		PostgresDSN:         os.Getenv("IOT_POSTGRES_DSN"),
		RedisAddr:           os.Getenv("IOT_REDIS_ADDR"),
		RedisPassword:       os.Getenv("IOT_REDIS_PASSWORD"),
		ClickHouseURL:       os.Getenv("IOT_CLICKHOUSE_URL"),
		MinIOEndpoint:       os.Getenv("IOT_MINIO_ENDPOINT"),
		MinIOAccessKey:      os.Getenv("IOT_MINIO_ACCESS_KEY"),
		MinIOSecretKey:      os.Getenv("IOT_MINIO_SECRET_KEY"),
		MinIOUseTLS:         boolValue("IOT_MINIO_USE_TLS", false),
		KafkaBrokers:        split(os.Getenv("IOT_KAFKA_BROKERS")),
		MQTTBroker:          os.Getenv("IOT_MQTT_BROKER"),
		MQTTUsername:        os.Getenv("IOT_MQTT_USERNAME"),
		MQTTPassword:        os.Getenv("IOT_MQTT_PASSWORD"),
		MQTTWebSocketURL:    os.Getenv("IOT_MQTT_WEBSOCKET_PUBLIC_URL"),
		OllamaURL:           get("IOT_OLLAMA_URL", "http://localhost:11434"),
		OllamaModel:         get("IOT_OLLAMA_MODEL", "qwen3:8b"),
		WeaviateURL:         os.Getenv("IOT_WEAVIATE_URL"),
		VideoSecrets:        parsePairs(os.Getenv("IOT_VIDEO_PLATFORM_SECRETS")),
		VideoMediaHosts:     split(os.Getenv("IOT_VIDEO_MEDIA_ALLOWED_HOSTS")),
		ThingsPanelURL:      strings.TrimRight(os.Getenv("IOT_THINGSPANEL_URL"), "/"),
		ThingsPanelUser:     os.Getenv("IOT_THINGSPANEL_USER"),
		ThingsPanelPassword: os.Getenv("IOT_THINGSPANEL_PASSWORD"),
		ThingsPanelSync:     duration("IOT_THINGSPANEL_SYNC_INTERVAL", 5*time.Minute),
		OfflineScan:         duration("IOT_OFFLINE_SCAN_INTERVAL", 30*time.Second),
		DevMode:             boolValue("IOT_DEV_MODE", true),
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
