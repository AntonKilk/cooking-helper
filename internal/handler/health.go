package handler

import (
	"context"
	"net/http"
	"time"
)

// pinger is the subset of *sql.DB the readiness probe needs.
type pinger interface {
	PingContext(ctx context.Context) error
}

const healthPingTimeout = 2 * time.Second

// Health returns the GET /healthz handler. It reports 200 when the database
// connection is reachable and 503 when it is not.
func Health(db pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), healthPingTimeout)
		defer cancel()

		w.Header().Set("Content-Type", "application/json")
		if err := db.PingContext(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unavailable"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}
