package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"iot-platform/internal/backup"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	service, err := backup.New(ctx, backup.Config{
		PostgresDSN: os.Getenv("IOT_POSTGRES_DSN"), BackupDir: env("IOT_BACKUP_DIR", "/app/data/backups"), BackupBucket: env("IOT_BACKUP_BUCKET", "iot-backups"),
		MinIOEndpoint: os.Getenv("IOT_MINIO_ENDPOINT"), MinIOAccessKey: os.Getenv("IOT_MINIO_ACCESS_KEY"), MinIOSecretKey: os.Getenv("IOT_MINIO_SECRET_KEY"), MinIOUseTLS: boolean("IOT_MINIO_USE_TLS"),
		MinIODREndpoint: os.Getenv("IOT_MINIO_DR_ENDPOINT"), MinIODRAccessKey: os.Getenv("IOT_MINIO_DR_ACCESS_KEY"), MinIODRSecretKey: os.Getenv("IOT_MINIO_DR_SECRET_KEY"), MinIODRUseTLS: boolean("IOT_MINIO_DR_USE_TLS"),
		ClickHouseURL: os.Getenv("IOT_CLICKHOUSE_URL"), RedisAddr: os.Getenv("IOT_REDIS_ADDR"), RedisPassword: os.Getenv("IOT_REDIS_PASSWORD"), RedpandaAdminURL: os.Getenv("IOT_REDPANDA_ADMIN_URL"), WeaviateURL: os.Getenv("IOT_WEAVIATE_URL"), ConfigPaths: os.Getenv("IOT_BACKUP_CONFIG_PATHS"),
	})
	if err != nil {
		log.Error("initialize backup service", "error", err)
		os.Exit(1)
	}
	defer service.Close()
	if boolean("IOT_BACKUP_ONCE") {
		kind := env("IOT_BACKUP_TYPE", "FULL")
		result, runErr := service.Run(ctx, kind)
		if runErr != nil {
			log.Error("one-shot backup failed", "type", kind, "error", runErr)
			os.Exit(1)
		}
		log.Info("one-shot backup completed", "type", kind, "id", result.ID)
		return
	}
	adminToken := env("IOT_BACKUP_ADMIN_TOKEN", "change-me-backup-admin-token")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "UP"})
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(service.Metrics()))
	})
	mux.HandleFunc("POST /backup", protected(adminToken, func(w http.ResponseWriter, r *http.Request) {
		result, runErr := service.Run(r.Context(), r.URL.Query().Get("type"))
		respond(w, result, runErr)
	}))
	mux.HandleFunc("POST /restore/drill", protected(adminToken, func(w http.ResponseWriter, r *http.Request) {
		result, runErr := service.Verify(r.Context(), r.URL.Query().Get("backupId"))
		respond(w, result, runErr)
	}))
	mux.HandleFunc("GET /backups", protected(adminToken, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		result, listErr := service.ListTasks(r.Context(), query.Get("type"), query.Get("status"), intQuery(query.Get("limit"), 50), intQuery(query.Get("offset"), 0))
		respond(w, result, listErr)
	}))
	mux.HandleFunc("GET /backups/{id}", protected(adminToken, func(w http.ResponseWriter, r *http.Request) {
		result, getErr := service.GetTask(r.Context(), r.PathValue("id"))
		respond(w, result, getErr)
	}))
	mux.HandleFunc("GET /backups/{id}/files", protected(adminToken, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		limit := intQuery(query.Get("pageSize"), intQuery(query.Get("limit"), 20))
		offset := intQuery(query.Get("offset"), 0)
		if page := intQuery(query.Get("page"), 0); page > 0 {
			offset = (page - 1) * limit
		}
		result, listErr := service.ListArtifactsPage(r.Context(), r.PathValue("id"), limit, offset)
		respond(w, result, listErr)
	}))
	mux.HandleFunc("GET /backups/{id}/files/{filename}", protected(adminToken, func(w http.ResponseWriter, r *http.Request) {
		object, artifact, openErr := service.OpenArtifact(r.Context(), r.PathValue("id"), r.PathValue("filename"))
		if openErr != nil {
			respond(w, nil, openErr)
			return
		}
		defer object.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, artifact.Filename))
		w.Header().Set("X-Checksum-SHA256", artifact.SHA256)
		if artifact.Size >= 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(artifact.Size, 10))
		}
		if _, copyErr := io.Copy(w, object); copyErr != nil {
			log.Error("backup artifact download failed", "backupId", r.PathValue("id"), "filename", artifact.Filename, "error", copyErr)
		}
	}))
	server := &http.Server{Addr: env("IOT_BACKUP_HTTP_ADDR", ":8090"), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if listenErr := server.ListenAndServe(); listenErr != nil && listenErr != http.ErrServerClosed {
			log.Error("backup HTTP service failed", "error", listenErr)
			cancel()
		}
	}()
	go scheduler(ctx, service, log, "FULL", duration("IOT_BACKUP_INTERVAL", 24*time.Hour), true)
	go scheduler(ctx, service, log, "INCREMENTAL", duration("IOT_BACKUP_INCREMENTAL_INTERVAL", 15*time.Minute), false)
	<-ctx.Done()
	shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	_ = server.Shutdown(shutdown)
}

func scheduler(ctx context.Context, service *backup.Service, log *slog.Logger, kind string, interval time.Duration, immediately bool) {
	if immediately {
		if result, err := service.Run(ctx, kind); err != nil {
			log.Error("scheduled backup failed", "type", kind, "error", err)
		} else {
			log.Info("scheduled backup completed", "type", kind, "id", result.ID)
		}
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if result, err := service.Run(ctx, kind); err != nil {
				log.Error("scheduled backup failed", "type", kind, "error", err)
			} else {
				log.Info("scheduled backup completed", "type", kind, "id", result.ID)
			}
		}
	}
}
func protected(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
func respond(w http.ResponseWriter, value any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(value)
}
func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
func duration(name string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(os.Getenv(name))
	if err != nil {
		return fallback
	}
	return value
}

func intQuery(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func boolean(name string) bool {
	value, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv(name)))
	return value
}
