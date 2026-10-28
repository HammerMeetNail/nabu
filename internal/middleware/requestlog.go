package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/HammerMeetNail/nabu/internal/clientip"
	"github.com/HammerMeetNail/nabu/internal/diagnostics"
)

// StatusClientClosedRequest is logged when the client disconnected before the
// handler finished. The handler's own error status (usually 500 from a
// canceled query) is not a server fault and must not count toward error rates.
const StatusClientClosedRequest = 499

// hashClientIP returns a short, stable fingerprint of the request's client IP
// so access logs can correlate a source without storing the raw address. The
// port is stripped first so the same client hashes consistently across requests.
func hashClientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	if host == "" || host == "unknown" {
		return ""
	}
	sum := sha256.Sum256([]byte(host))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:12]
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// RequestLogger logs one record per request ("request"), a JSON line with the
// server's handler. trustedProxies are the TRUSTED_PROXY_CIDRS prefixes:
// behind a proxy or tunnel the client hash is taken from the forwarded client
// address, not the proxy's. A nil logger means slog.Default() when the request
// is logged.
func RequestLogger(logger *slog.Logger, trustedProxies ...netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now().UTC()
			requestID := diagnostics.RequestID()
			w.Header().Set("X-Request-ID", requestID)
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)

			status := recorder.status
			if status >= 500 && errors.Is(r.Context().Err(), context.Canceled) {
				status = StatusClientClosedRequest
			}
			out := logger
			if out == nil {
				out = slog.Default()
			}
			out.LogAttrs(r.Context(), slog.LevelInfo, "request",
				slog.String("request_id", requestID),
				slog.String("ts", start.Format(time.RFC3339)),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				// Log a stable hash of the client IP rather than the raw address:
				// it still lets operators correlate requests from one source
				// (rate-limit abuse, error bursts) without retaining PII in logs.
				slog.String("client", hashClientIP(clientip.Address(r, trustedProxies))),
			)
		})
	}
}
