package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/HammerMeetNail/nabu/internal/version"
)

const ReadinessTimeout = time.Second

type HealthResponse struct {
	Status string `json:"status"`
	// Version is the release running here, which the static asset URLs
	// already show. The iOS release workflow reads it to check that the web
	// app an iOS build needs is live.
	Version string `json:"version,omitempty"`
}

func Health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok", Version: version.Version})
}

func Ready(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ready"})
}

// Readiness probes the same pool used by handlers, so pool exhaustion and a
// lost database both remove readiness. Cancellation limits each probe to 1s.
// A nil probe is only used by the explicitly configured in-memory dev server.
func Readiness(probe func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if probe != nil {
			ctx, cancel := context.WithTimeout(r.Context(), ReadinessTimeout)
			defer cancel()
			if err := probe(ctx); err != nil {
				id := diagnoseError(w, "database readiness", err)
				w.Header().Set("Retry-After", "1")
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "requestId": id})
				return
			}
		}
		Ready(w, r)
	}
}
