package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	storageclient "mrsydar/tagbase/storage/pkg/client"
	"mrsydar/tagbase/tagger/internal/server"
	"mrsydar/tagbase/tagger/pkg/evaluator"
)

func main() {
	httpAddr := os.Getenv("TAGGER_HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8081"
	}
	storageBaseURL := os.Getenv("TAGGER_STORAGE_BASE_URL")
	if storageBaseURL == "" {
		storageBaseURL = "http://localhost:8080"
	}
	evaluatorImpl := os.Getenv("TAGGER_EVALUATOR_IMPL")
	if evaluatorImpl == "" {
		evaluatorImpl = "false"
	}

	var ev evaluator.Evaluator
	switch evaluatorImpl {
	case "grep":
		ev = evaluator.NewGrepEvaluator()
	case "false":
		ev = evaluator.NewFalseEvaluator()
	case "openai":
		openaiAPIKey := os.Getenv("TAGGER_OPENAI_API_KEY")
		openaiBaseURL := os.Getenv("TAGGER_OPENAI_BASE_URL")
		if openaiBaseURL == "" {
			openaiBaseURL = "https://api.openai.com/v1"
		}
		openaiModel := os.Getenv("TAGGER_OPENAI_MODEL")
		if openaiModel == "" {
			openaiModel = "gpt-4o-mini"
		}
		ev = evaluator.NewOpenAIEvaluator(openaiAPIKey, openaiBaseURL, openaiModel)
	default:
		fmt.Fprintf(os.Stderr, "unknown evaluator implementation: %s\n", evaluatorImpl)
		os.Exit(1)
	}

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	storageClient := storageclient.New(storageBaseURL)
	srv := server.NewServer(storageClient, ev, logger)

	httpServer := &http.Server{
		Addr:         httpAddr,
		Handler:      srv.Router(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		logger.Info("starting tagger server", zap.String("addr", httpAddr), zap.String("storage_url", storageBaseURL))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("http server error", zap.Error(err))
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpServer.Shutdown(shutdownCtx)
	logger.Info("shutdown complete")
}
