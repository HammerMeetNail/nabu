package mail

import (
	"context"
	"testing"
)

func TestSkipInvalidDropsOnlyUndeliverableMail(t *testing.T) {
	memory := NewMemorySender()
	sender := SkipInvalid(memory)
	for _, to := range []string{
		"smoke-20261029@nabu-smoke.invalid",
		"someone@example.invalid",
		"Someone <SOMEONE@Example.INVALID>",
		"trailing@nabu.invalid.",
		"bare@invalid",
	} {
		if err := sender.Send(context.Background(), Message{To: to}); err != nil {
			t.Fatalf("send to %s: %v", to, err)
		}
	}
	if got := memory.Messages(); len(got) != 0 {
		t.Fatalf("mail to .invalid addresses was sent: %+v", got)
	}

	for _, to := range []string{"person@example.com", "verify+smoke@nabu-app.com", "invalid@example.org", "x@notinvalid"} {
		if err := sender.Send(context.Background(), Message{To: to}); err != nil {
			t.Fatalf("send to %s: %v", to, err)
		}
	}
	if got := memory.Messages(); len(got) != 4 {
		t.Fatalf("deliverable mail sent = %d, want 4: %+v", len(got), got)
	}
}
