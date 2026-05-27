package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	dict "github.com/AntonKilk/cooking-helper/i18n"
	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/handler"
	"github.com/AntonKilk/cooking-helper/internal/i18n"
	"github.com/AntonKilk/cooking-helper/internal/llm"
	"github.com/AntonKilk/cooking-helper/internal/llm/anthropic"
	"github.com/AntonKilk/cooking-helper/internal/llm/openai"
	"github.com/AntonKilk/cooking-helper/internal/repository"
	"github.com/AntonKilk/cooking-helper/static"
	"github.com/AntonKilk/cooking-helper/templates"
)

const (
	defaultPort     = "8080"
	defaultDBPath   = "data/cooking.db"
	shutdownTimeout = 10 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("server exited with error", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = defaultDBPath
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}

	db, err := repository.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := repository.RunMigrations(db); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	logger.Info("database ready", "path", dbPath)

	bundle, err := i18n.Load(dict.FS, domain.LanguageEN)
	if err != nil {
		return fmt.Errorf("load i18n: %w", err)
	}

	tmpl, err := template.New("").Funcs(handler.ParseFuncMap()).ParseFS(templates.FS, "*.gohtml")
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}

	llmClient := newLLMClient(logger)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler.NewRouter(logger, db, bundle, tmpl, static.FS, llmClient),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Trap SIGINT/SIGTERM so we can shut down gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("listen and serve: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	logger.Info("server stopped")
	return nil
}

// newLLMClient selects the LLM provider from the environment: Anthropic when
// ANTHROPIC_API_KEY is set, otherwise OpenAI when OPENAI_API_KEY is set. When
// neither is present, generation is disabled and nil is returned (the rest of the
// app runs). The SDK base URL is deliberately left at its pinned default and is
// never read from the environment (see CLAUDE.md › Security).
func newLLMClient(logger *slog.Logger) llm.Client {
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		logger.Info("llm provider configured", "provider", "anthropic")
		return anthropic.New(key, anthropic.WithLogger(logger))
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		logger.Info("llm provider configured", "provider", "openai")
		return openai.New(key, openai.WithLogger(logger))
	}
	logger.Warn("no LLM API key set; weekly generation disabled")
	return nil
}
