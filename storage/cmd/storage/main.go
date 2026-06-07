package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"mrsydar/tagbase/storage/internal/config"
	"mrsydar/tagbase/storage/internal/db"
	"mrsydar/tagbase/storage/internal/retention"
	"mrsydar/tagbase/storage/internal/server"
	"mrsydar/tagbase/storage/internal/storage"
	taggerclient "mrsydar/tagbase/tagger/pkg/client"
)

func main() {
	cfg, err := config.Load("TAGBASE_")
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Postgres.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := pgxpool.New(ctx, cfg.PGDSN)
	cancel()
	if err != nil {
		logger.Fatal("pgx pool error", zap.Error(err))
	}
	defer pool.Close()

	// Run migrations.
	if err := runMigrations(pool); err != nil {
		logger.Fatal("migrations failed", zap.Error(err))
	}

	// S3.
	store, err := storage.NewS3Store(cfg.S3Endpoint, cfg.S3Region, cfg.S3Bucket, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3ForcePathStyle)
	if err != nil {
		logger.Fatal("s3 store error", zap.Error(err))
	}
	for i := 0; i < 10; i++ {
		if err := store.EnsureBucket(context.Background()); err != nil {
			if i == 9 {
				logger.Fatal("ensure bucket failed after retries", zap.Error(err))
			}
			logger.Warn("ensure bucket failed, retrying", zap.Error(err), zap.Int("attempt", i+1))
			time.Sleep(2 * time.Second)
			continue
		}
		break
	}

	// Tag engine client.
	if cfg.TagEngineURL == "" {
		logger.Fatal("TAGBASE_TAG_ENGINE_URL is required")
	}
	tagClient := taggerclient.New(cfg.TagEngineURL)

	// Fetch supported types with retries.
	var supportedTypes []string
	for i := 0; i < 10; i++ {
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		supportedTypes, err = tagClient.GetSupportedTypes(ctx)
		cancel()
		if err == nil {
			break
		}
		if i == 9 {
			logger.Fatal("failed to fetch supported types from tagging engine", zap.Error(err))
		}
		logger.Warn("fetch supported types failed, retrying", zap.Error(err), zap.Int("attempt", i+1))
		time.Sleep(2 * time.Second)
	}
	logger.Info("supported data types", zap.Strings("types", supportedTypes))

	database := db.New(pool)
	srv := server.NewServer(cfg, database, store, tagClient, logger)
	srv.SetSupportedTypes(supportedTypes)

	// Retention sweeper.
	sweeper := retention.NewSweeper(database, store, cfg.RetentionSweepInterval, logger)
	sweeperCtx, sweeperCancel := context.WithCancel(context.Background())
	go sweeper.Start(sweeperCtx)

	// HTTP server.
	httpAddr := cfg.HTTPAddr
	httpServer := &http.Server{
		Addr:         httpAddr,
		Handler:      srv.Router(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		logger.Info("starting tagbase server", zap.String("addr", httpAddr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("http server error", zap.Error(err))
		}
	}()

	// Graceful shutdown.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	logger.Info("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	sweeperCancel()
	httpServer.Shutdown(shutdownCtx)
	logger.Info("shutdown complete")
}

func runMigrations(pool *pgxpool.Pool) error {
	// Simple migration runner: execute all .up.sql files in migrations/.
	// In production, use golang-migrate. For MVP, we assume schema is idempotent.
	ctx := context.Background()
	files, err := os.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".up.sql") {
			continue
		}
		data, err := os.ReadFile(filepath.Join("migrations", f.Name()))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", f.Name(), err)
		}
		if _, err := pool.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("exec migration %s: %w", f.Name(), err)
		}
	}
	return nil
}
