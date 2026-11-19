// Command api é o composition root da API: lê configuração, monta os adapters
// e injeta nos use cases (DI manual, sem framework).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/events"
	httpapi "github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/http"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/ids"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/postgres"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/schema"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/storage"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/worker"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/application"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

type config struct {
	HTTPAddr      string
	DatabaseURL   string
	MediaDir      string
	InternalToken string
	CORSOrigin    string
	// StoryboardWorkers é o número de gerações de roteiro em paralelo.
	StoryboardWorkers int
	// Rate limits por IP (requisições por minuto).
	CreatePerMinute float64
	RenderPerMinute float64
	// MediaOrphanRetention: idade mínima para apagar vídeos que não são mais o atual de nenhum job.
	MediaOrphanRetention time.Duration
}

func loadConfig() (config, error) {
	cfg := config{
		HTTPAddr:      envOr("HTTP_ADDR", ":8080"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		MediaDir:      envOr("MEDIA_DIR", "media"),
		InternalToken: os.Getenv("INTERNAL_TOKEN"),
		CORSOrigin:    envOr("CORS_ORIGIN", "http://localhost:3000"),
	}
	var err error
	if cfg.CreatePerMinute, err = strconv.ParseFloat(envOr("RATE_LIMIT_CREATE_PER_MIN", "10"), 64); err != nil || cfg.CreatePerMinute <= 0 {
		return cfg, fmt.Errorf("RATE_LIMIT_CREATE_PER_MIN inválido")
	}
	if cfg.RenderPerMinute, err = strconv.ParseFloat(envOr("RATE_LIMIT_RENDER_PER_MIN", "6"), 64); err != nil || cfg.RenderPerMinute <= 0 {
		return cfg, fmt.Errorf("RATE_LIMIT_RENDER_PER_MIN inválido")
	}
	if cfg.MediaOrphanRetention, err = time.ParseDuration(envOr("MEDIA_ORPHAN_RETENTION", "168h")); err != nil || cfg.MediaOrphanRetention < time.Hour {
		return cfg, fmt.Errorf("MEDIA_ORPHAN_RETENTION inválido (mínimo 1h)")
	}
	if n, err := strconv.Atoi(envOr("STORYBOARD_WORKERS", "2")); err != nil || n <= 0 {
		return cfg, fmt.Errorf("STORYBOARD_WORKERS inválido")
	} else {
		cfg.StoryboardWorkers = n
	}
	var missing []string
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if cfg.InternalToken == "" {
		missing = append(missing, "INTERNAL_TOKEN")
	}
	if len(missing) > 0 {
		return cfg, fmt.Errorf("variáveis obrigatórias ausentes: %v", missing)
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("api encerrada com erro", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	llmCfg, err := loadLLMConfig(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := postgres.Migrate(ctx, db); err != nil {
		return fmt.Errorf("migrações: %w", err)
	}

	catalog, err := schema.NewEmbeddedCatalog()
	if err != nil {
		return fmt.Errorf("catálogo de templates: %w", err)
	}
	media, err := storage.NewLocalFS(cfg.MediaDir)
	if err != nil {
		return err
	}
	broker := events.NewBroker()

	generator, err := buildGenerator(llmCfg, catalog, postgres.NewUsageRecorder(db), logger)
	if err != nil {
		return fmt.Errorf("llm: %w", err)
	}
	logger.Info("provedor de roteiro", "provider", llmCfg.Provider)

	svc := application.NewService(application.Deps{
		UoW:       postgres.NewUnitOfWork(db),
		Generator: generator,
		Catalog:   catalog,
		Events:    broker,
		IDs:       ids.Random{},
		Clock:     ids.SystemClock{},
	})

	// Workers em goroutines: roteiro (LLM, I/O-bound) e manutenção da fila.
	queue := postgres.NewQueue(db)
	listener, err := postgres.NewListener(cfg.DatabaseURL, ports.TaskGenerateStoryboard, logger)
	if err != nil {
		return err
	}
	defer listener.Close()
	hostname, _ := os.Hostname()
	var workers sync.WaitGroup
	workers.Add(3)
	go func() {
		defer workers.Done()
		worker.MediaCleanup{
			Files:        media,
			References:   postgres.NewMediaReferences(db),
			PartialAfter: 24 * time.Hour,
			OrphanAfter:  cfg.MediaOrphanRetention,
			Logger:       logger,
		}.Run(ctx, time.Hour)
	}()
	go func() {
		defer workers.Done()
		worker.NewStoryboardWorker(queue, svc, listener, worker.StoryboardConfig{
			WorkerID:    "storyboard@" + hostname,
			Concurrency: cfg.StoryboardWorkers,
		}, logger).Run(ctx)
	}()
	go func() {
		defer workers.Done()
		worker.Maintenance{Queue: queue, Interval: time.Minute, StaleAfter: 5 * time.Minute, Logger: logger}.Run(ctx)
	}()
	defer workers.Wait() // graceful: aguarda tarefas em andamento

	server, err := httpapi.NewServer(svc, broker, media, httpapi.Config{
		InternalToken: cfg.InternalToken,
		AllowedOrigin: cfg.CORSOrigin,
		CreateLimit:   &httpapi.RateLimit{PerMinute: cfg.CreatePerMinute, Burst: 3},
		RenderLimit:   &httpapi.RateLimit{PerMinute: cfg.RenderPerMinute, Burst: 2},
		Metrics:       postgres.NewMetricsReader(db),
	}, logger)
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// Sem WriteTimeout: streams SSE e downloads de vídeo são longos.
		IdleTimeout: 120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("api ouvindo", "addr", cfg.HTTPAddr, "media_dir", cfg.MediaDir)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	logger.Info("encerrando api")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}
