package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	aiadapter "iot-platform/internal/adapters/ai"
	clickhouseadapter "iot-platform/internal/adapters/clickhouse"
	kafkaadapter "iot-platform/internal/adapters/kafka"
	"iot-platform/internal/adapters/knowledge"
	"iot-platform/internal/adapters/local"
	"iot-platform/internal/adapters/memory"
	minioadapter "iot-platform/internal/adapters/minio"
	mqttadapter "iot-platform/internal/adapters/mqtt"
	"iot-platform/internal/adapters/postgres"
	redisadapter "iot-platform/internal/adapters/redis"
	thingspaneladapter "iot-platform/internal/adapters/thingspanel"
	videoadapter "iot-platform/internal/adapters/video"
	"iot-platform/internal/auth"
	"iot-platform/internal/config"
	"iot-platform/internal/core"
	"iot-platform/internal/httpapi"
	"iot-platform/internal/metrics"
	"iot-platform/internal/model"
	"iot-platform/internal/parser"
	"iot-platform/internal/ports"
)

func main() {
	cfg := config.Load()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var repo ports.Repository = memory.NewRepository()
	if cfg.PostgresDSN != "" {
		r, err := postgres.New(ctx, cfg.PostgresDSN)
		fatal(log, "initialize postgres", err)
		repo = r
		log.Info("repository enabled", "adapter", "postgres")
	}
	if cfg.ClickHouseURL != "" {
		r, clickErr := clickhouseadapter.New(ctx, cfg.ClickHouseURL, repo)
		fatal(log, "initialize clickhouse", clickErr)
		repo = r
		log.Info("telemetry storage enabled", "adapter", "clickhouse")
	}
	if cfg.RedisAddr != "" {
		repo = redisadapter.New(repo, cfg.RedisAddr, cfg.RedisPassword)
		log.Info("hot state cache enabled", "adapter", "redis")
	}
	var archivePort ports.Archive
	if cfg.MinIOEndpoint != "" {
		m, err := minioadapter.New(cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey, cfg.MinIOUseTLS)
		fatal(log, "initialize minio", err)
		archivePort = m
		log.Info("archive enabled", "adapter", "minio")
	} else {
		archive, err := local.NewArchive(filepath.Join(cfg.DataDir, "objects"))
		fatal(log, "initialize local archive", err)
		archivePort = archive
	}
	var bus ports.EventBus = local.NewBus()
	if len(cfg.KafkaBrokers) > 0 {
		bus = kafkaadapter.New(cfg.KafkaBrokers)
		log.Info("event bus enabled", "adapter", "kafka", "brokers", cfg.KafkaBrokers)
	}
	localRealtime := local.NewRealtime()
	var realtime ports.RealtimePublisher = localRealtime
	registry := metrics.New()
	var mqttClient *mqttadapter.Client
	if cfg.MQTTBroker != "" {
		credentials := func() (string, string) { return cfg.MQTTUsername, cfg.MQTTPassword }
		if cfg.MQTTPassword == "" {
			manager := auth.New(cfg.JWTSecret)
			acl := []auth.ACLRule{
				{Permission: "allow", Action: "subscribe", Topic: "/external/raw/#"},
				{Permission: "allow", Action: "subscribe", Topic: "/jetlinks/raw/#"},
				{Permission: "allow", Action: "subscribe", Topic: "/external/video/alarm/#"},
				{Permission: "allow", Action: "subscribe", Topic: "/iot/device/state/#"},
				{Permission: "allow", Action: "publish", Topic: "/iot/#"},
			}
			credentials = func() (string, string) {
				token, err := manager.IssueWithACL("iot-platform", "system", "service", nil, acl, time.Hour)
				if err != nil {
					log.Error("issue mqtt service token", "error", err)
					return "iot-platform", ""
				}
				return "iot-platform", token
			}
		}
		mqttConnection, err := mqttadapter.NewWithCredentials(cfg.MQTTBroker, "iot-platform-"+hostname(), credentials)
		fatal(log, "connect mqtt", err)
		mqttClient = mqttConnection
		realtime = mqttClient
		registry.Set("mqtt_subscription_count", 4)
		log.Info("realtime enabled", "adapter", "mqtt")
	}
	parsers := parser.NewRegistry(parser.GB26875Parser{}, parser.ConfigurableJSONParser{}, parser.ConfigurableHexParser{}, parser.ModbusCoilParser{}, parser.JavaScriptParser{}, parser.ExternalParser{Root: cfg.DataDir}, parser.FireSmokeHexParser{}, parser.ModbusParser{}, parser.JSONParser{})
	engine := core.New(repo, archivePort, bus, realtime, parsers, log)
	engine.VideoMediaAllowedHosts = cfg.VideoMediaHosts
	engine.RequireVideoCameraMapping = !cfg.DevMode
	var hikvisionResolver videoadapter.StreamResolver
	if cfg.HikvisionVideoAPIURL != "" || cfg.HikvisionAppKey != "" || cfg.HikvisionAppSecret != "" {
		resolver, resolverErr := videoadapter.NewHikvisionArtemis(videoadapter.HikvisionArtemisConfig{
			BaseURL: cfg.HikvisionVideoAPIURL, AppKey: cfg.HikvisionAppKey, AppSecret: cfg.HikvisionAppSecret,
		})
		fatal(log, "initialize official Hikvision Go adapter", resolverErr)
		hikvisionResolver = resolver
	}
	videoPreview, videoPreviewErr := videoadapter.New(videoadapter.Config{
		ZLMAPIURL: cfg.VideoZLMAPIURL, ZLMPlaybackBaseURL: cfg.VideoZLMPlaybackURL, ZLMSecret: cfg.VideoZLMSecret,
		ZLMVhost: cfg.VideoZLMVhost, ZLMApp: cfg.VideoZLMApp,
		DahuaSDKURL: cfg.DahuaVideoSDKURL, DahuaSDKToken: cfg.DahuaVideoSDKToken,
		HikvisionAPIURL: cfg.HikvisionVideoAPIURL, HikvisionResolver: hikvisionResolver,
		AllowedSourceHosts: cfg.VideoMediaHosts,
	})
	fatal(log, "initialize video preview", videoPreviewErr)
	engine.VideoPreview = videoPreview
	engine.Metrics = registry
	aiPlugins := aiadapter.NewProviderRegistry()
	engine.AIPlugins = aiPlugins
	providerID := cfg.AIProvider
	if providerID == "" && os.Getenv("IOT_OLLAMA_URL") != "" {
		providerID = "ollama"
	}
	providerConfig := ports.AIPluginConfig{Provider: providerID, BaseURL: cfg.AIBaseURL, Model: cfg.AIModel, APIKey: cfg.AIAPIKey}
	if providerID == "ollama" {
		if providerConfig.BaseURL == "" {
			providerConfig.BaseURL = cfg.OllamaURL
		}
		if providerConfig.Model == "" {
			providerConfig.Model = cfg.OllamaModel
		}
	}
	baseAI, providerErr := aiPlugins.Create(providerConfig)
	fatal(log, "initialize AI provider plugin", providerErr)
	einoAI, einoErr := aiadapter.NewEino(ctx, baseAI)
	fatal(log, "initialize Eino AI workflows", einoErr)
	engine.AI = einoAI
	if cfg.AIHarnessURL != "" {
		harness, harnessErr := aiadapter.NewHarness(cfg.AIHarnessURL, cfg.AIHarnessToken, cfg.AIHarnessMCPURL, cfg.AIHarnessModel, cfg.AIHarnessTimeout)
		fatal(log, "initialize AI workflow harness", harnessErr)
		engine.AIWorkflows = harness
		log.Info("AI workflow harness enabled", "url", cfg.AIHarnessURL, "model", cfg.AIHarnessModel)
	}
	if cfg.WeaviateURL != "" {
		engine.KB = knowledge.NewWeaviate(cfg.WeaviateURL)
	} else {
		engine.KB = knowledge.NewLocal()
	}
	if cfg.ThingsPanelURL != "" {
		engine.Catalog = thingspaneladapter.New(cfg.ThingsPanelURL, cfg.ThingsPanelUser, cfg.ThingsPanelPassword, repo)
		go func() {
			ticker := time.NewTicker(cfg.ThingsPanelSync)
			defer ticker.Stop()
			for {
				if _, err := engine.Catalog.Sync(ctx, ""); err != nil {
					log.Warn("ThingsPanel catalog sync failed", "error", err)
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}
	fatal(log, "start engine", engine.Start(ctx))
	if mqttClient != nil {
		fatal(log, "subscribe raw mqtt", mqttClient.SubscribeRaw(func(c context.Context, v model.RawMessage) error { _, _, err := engine.IngestRaw(c, v); return err }))
		fatal(log, "subscribe device state mqtt", mqttClient.SubscribeDeviceState(engine.UpdateDeviceState))
		fatal(log, "subscribe video mqtt", mqttClient.SubscribeVideo(func(c context.Context, v model.VideoAlarmEvent) error {
			_, _, err := engine.IngestVideo(c, v)
			return err
		}))
	}
	api := httpapi.New(cfg, engine, registry, log)
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: api.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 15 * time.Minute, IdleTimeout: 2 * time.Minute}
	go func() {
		ticker := time.NewTicker(cfg.OfflineScan)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := engine.ScanOffline(ctx); err != nil {
					log.Error("offline scan failed", "error", err)
				}
			}
		}
	}()
	go func() {
		log.Info("iot platform started", "addr", cfg.HTTPAddr, "devMode", cfg.DevMode)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server failed", "error", err)
			cancel()
		}
	}()
	<-ctx.Done()
	shutdown, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	_ = server.Shutdown(shutdown)
	_ = realtime.Close()
	_ = bus.Close()
	_ = repo.Close()
	log.Info("iot platform stopped")
}
func fatal(log *slog.Logger, msg string, err error) {
	if err != nil {
		log.Error(msg, "error", err)
		os.Exit(1)
	}
}
func hostname() string {
	v, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return v
}
