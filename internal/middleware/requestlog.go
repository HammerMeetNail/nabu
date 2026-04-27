package middleware

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func RequestLogger(logger *log.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = log.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now().UTC()
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)

			payload, _ := json.Marshal(map[string]any{
				"ts":          start.Format(time.RFC3339),
				"method":      r.Method,
				"path":        r.URL.Path,
				"status":      recorder.status,
				"duration_ms": time.Since(start).Milliseconds(),
				"remote_addr": r.RemoteAddr,
			})
			logger.Println(string(payload))
		})
	}
}
