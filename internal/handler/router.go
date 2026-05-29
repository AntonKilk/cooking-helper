package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/AntonKilk/cooking-helper/internal/i18n"
	"github.com/AntonKilk/cooking-helper/internal/llm"
	"github.com/AntonKilk/cooking-helper/internal/repository"
	"github.com/AntonKilk/cooking-helper/internal/service"
)

type contextKey string

// requestIDKey is the context key under which the per-request ID is stored.
const requestIDKey contextKey = "request_id"

// NewRouter builds the application's HTTP handler: the route table wrapped in
// language-resolution, request-ID, and structured-logging middleware. The db
// backs the readiness probe; the bundle and tmpl drive localized rendering;
// staticFS serves the embedded front-end assets, manifest, and Service Worker.
// llmClient enables weekly generation; when nil, the feature is disabled and the
// home screen renders the button inert (the rest of the app works unchanged).
func NewRouter(logger *slog.Logger, db *sql.DB, bundle *i18n.Bundle, tmpl *template.Template, staticFS fs.FS, llmClient llm.Client) http.Handler {
	rd := &renderer{tmpl: tmpl, bundle: bundle}
	store := repository.New(db)
	svc := service.NewHouseholdService(store)
	ph := &profileHandlers{rd: rd, bundle: bundle, svc: svc}
	canGenerate := llmClient != nil
	hh := &homeHandlers{rd: rd, canGenerate: canGenerate}
	rh := &recipeHandlers{rd: rd, recipes: store, feedback: service.NewRecipeService(store)}
	sh := &shoppingHandlers{rd: rd, store: store, households: svc}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", Health(db))
	mux.HandleFunc("GET /{$}", hh.Home)
	mux.HandleFunc("GET /recipe/{id}", rh.Show)
	mux.HandleFunc("POST /recipe/{id}/feedback", rh.Feedback)
	mux.HandleFunc("GET /shopping", sh.List)
	mux.HandleFunc("POST /shopping/item/{id}/check", sh.Check)
	mux.HandleFunc("POST /shopping/item/{id}/remove", sh.Remove)
	mux.HandleFunc("POST /shopping/item/{id}/restore", sh.Restore)
	mux.HandleFunc("GET /settings", rd.Settings)
	mux.HandleFunc("POST /settings/language", SetLanguage(bundle))
	mux.HandleFunc("GET /settings/profile", ph.Show)
	mux.HandleFunc("POST /settings/profile", ph.Save)
	mux.Handle("GET /static/", StaticFiles(staticFS))
	mux.HandleFunc("GET /sw.js", ServiceWorker(staticFS))
	mux.HandleFunc("GET /manifest.webmanifest", Manifest(staticFS))

	if canGenerate {
		gh := &generateHandlers{
			rd:         rd,
			households: svc,
			gen:        service.NewGenerationService(llmClient, store, service.NewShoppingBuilder(llmClient, store)),
			recipes:    store,
		}
		mux.HandleFunc("POST /generate", gh.Generate)
		mux.HandleFunc("POST /generate/swap/{recipeID}", gh.Swap)
	}

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
