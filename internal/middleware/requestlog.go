package middleware

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"time"
)

// hashClientIP returns a short, stable fingerprint of the request's client IP
// so access logs can correlate a source without storing the raw address. The
// port is stripped first so the same client hashes consistently across requests.
func hashClientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	if host == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(host))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:12]
}

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
				// Log a stable hash of the client IP rather than the raw address:
				// it still lets operators correlate requests from one source
				// (rate-limit abuse, error bursts) without retaining PII in logs.
				"client": hashClientIP(r.RemoteAddr),
			})
			logger.Println(string(payload))
		})
	}
}
