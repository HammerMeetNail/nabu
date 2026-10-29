package lifecycle

import (
	"fmt"
	"testing"
	"time"
)

func TestSessionIdleSQLMatchesTimeout(t *testing.T) {
	want := fmt.Sprintf("INTERVAL '%d days'", SessionIdleTimeout/(24*time.Hour))
	if SessionIdleTimeout%(24*time.Hour) != 0 || SessionIdleSQL != want {
		t.Fatalf("SessionIdleSQL = %q, want %q for SessionIdleTimeout %v", SessionIdleSQL, want, SessionIdleTimeout)
	}
	if SessionLifetime <= SessionIdleTimeout {
		t.Fatalf("SessionLifetime %v must exceed SessionIdleTimeout %v", SessionLifetime, SessionIdleTimeout)
	}
}
