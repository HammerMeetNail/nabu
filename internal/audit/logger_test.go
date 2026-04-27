package audit

import (
	"bytes"
	"context"
	"log"
	"testing"
)

func TestStdLoggerWritesJSONEvent(t *testing.T) {
	var output bytes.Buffer
	logger := NewStdLogger(log.New(&output, "", 0))
	logger.Log(context.Background(), "auth.login_succeeded", map[string]string{
		"method":  "password",
		"user_id": "user-123",
	})

	body := output.String()
	if body == "" {
		t.Fatal("expected audit log output")
	}
	if !bytes.Contains([]byte(body), []byte(`"event":"auth.login_succeeded"`)) {
		t.Fatalf("body = %s", body)
	}
	if !bytes.Contains([]byte(body), []byte(`"method":"password"`)) {
		t.Fatalf("body = %s", body)
	}
	if !bytes.Contains([]byte(body), []byte(`"user_id":"user-123"`)) {
		t.Fatalf("body = %s", body)
	}
}
