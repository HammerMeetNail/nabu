package handlers

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestServerErrorLogsOnlyClassAndCorrelation(t *testing.T) {
	for _, tc := range []struct {
		name, class string
		err         error
	}{
		{"transport", "timeout", &url.Error{Op: "Post", URL: "https://provider.invalid/CAPABILITY-SECRET?token=SECRET-TOKEN", Err: context.DeadlineExceeded}},
		{"database", "sql_23503", &pgconn.PgError{Code: "23503", Message: "PRIVATE-EMAIL@example.invalid", Detail: "PRIVATE-HOUSEHOLD"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var captured bytes.Buffer
			old := log.Writer()
			log.SetOutput(&captured)
			defer log.SetOutput(old)
			w := httptest.NewRecorder()
			w.Header().Set("X-Request-ID", "synthetic-request-id")
			writeServerError(w, "failed to delete account", fmt.Errorf("wrapped: %w", tc.err))
			both := w.Body.String() + captured.String()
			for _, secret := range []string{"CAPABILITY-SECRET", "SECRET-TOKEN", "PRIVATE-EMAIL", "PRIVATE-HOUSEHOLD"} {
				if strings.Contains(both, secret) {
					t.Fatal("private error content escaped diagnostic boundary")
				}
			}
			if w.Code != 500 || !strings.Contains(w.Body.String(), "synthetic-request-id") || !strings.Contains(captured.String(), "request_id=synthetic-request-id") || !strings.Contains(captured.String(), "error_class="+tc.class) {
				t.Fatal("lost status, correlation, or sanitized error class")
			}
		})
	}
}
