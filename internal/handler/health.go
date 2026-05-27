package handler

import (
	"net/http"
)

// Health responds to GET /healthz. It currently returns 200 unconditionally as
// a readiness stub; CH-3 wires in a real DB-connection check (200 ready / 503 not).
func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
