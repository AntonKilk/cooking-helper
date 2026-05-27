package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/AntonKilk/cooking-helper/internal/i18n"
	"github.com/AntonKilk/cooking-helper/internal/repository"
	"github.com/AntonKilk/cooking-helper/internal/service"
)

type contextKey string

// requestIDKey is the context key under which the per-request ID is stored.
const requestIDKey contextKey = "request_id"

// NewRouter builds the application's HTTP handler: the route table wrapped in
// language-resolution, request-ID, and structured-logging middleware. The db
// backs the readiness probe; the bundle and tmpl drive localized rendering.
func NewRouter(logger *slog.Logger, db *sql.DB, bundle *i18n.Bundle, tmpl *template.Template) http.Handler {
	rd := &renderer{tmpl: tmpl, bundle: bundle}
	svc := service.NewHouseholdService(repository.New(db))
	ph := &profileHandlers{rd: rd, bundle: bundle, svc: svc}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", Health(db))
	mux.HandleFunc("GET /{$}", rd.Home)
	mux.HandleFunc("POST /settings/language", SetLanguage(bundle))
	mux.HandleFunc("GET /settings/profile", ph.Show)
	mux.HandleFunc("POST /settings/profile", ph.Save)

	return requestLogger(logger, languageMiddleware(bundle, mux))
}

// RequestIDFromContext returns the request ID attached by the middleware, if any.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// requestLogger assigns a request ID, logs the incoming request, and propagates
// the ID via the request context for downstream use (e.g. LLM calls).
func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		ctx := context.WithValue(r.Context(), requestIDKey, id)

		logger.Info("request received",
			"method", r.Method,
			"path", r.URL.Path,
			"request_id", id,
		)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// newRequestID returns a random 128-bit hex token. crypto/rand never fails on
// the platforms we target, so a read error degrades to an empty ID rather than
// taking down the request.
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}
