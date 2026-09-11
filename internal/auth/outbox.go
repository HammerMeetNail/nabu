package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/HammerMeetNail/nabu/internal/mail"
)

func (s *Service) queueVerificationEmail(ctx context.Context, user User) error {
	token, err := s.createToken(ctx, &user.ID, user.Email, "verify", 24*time.Hour)
	if err != nil {
		return err
	}
	return s.store.QueueAuthMail(ctx, user.ID, mail.Message{To: user.Email,
		Subject: "Verify your Nabu email", Body: emailVerificationTemplate(s.baseURL, token)}, s.now().Add(24*time.Hour))
}

// DeliverPendingMail is bounded, safe for multiple processes, and cancelable.
// A lease permits a retry after a process crash. SMTP can accept a message just
// before a crash, so occasional duplicate emails are possible; tokens are one use.
func (s *Service) DeliverPendingMail(ctx context.Context) {
	for i := 0; i < 10 && ctx.Err() == nil; i++ {
		now := s.now()
		job, err := s.store.ClaimAuthMail(ctx, now, now.Add(time.Minute), randomToken(24))
		if errors.Is(err, sql.ErrNoRows) {
			return
		}
		if err != nil {
			s.logAudit(ctx, "auth.mail_queue_failed", map[string]string{"operation": "claim"})
			return
		}
		deliveryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = s.mailer.Send(deliveryCtx, job.Message)
		cancel()
		delay := min(time.Duration(1<<min(job.Attempts, 5))*30*time.Second, 15*time.Minute)
		if finishErr := s.store.FinishAuthMail(ctx, job, err == nil, now.Add(delay)); finishErr != nil {
			s.logAudit(ctx, "auth.mail_queue_failed", map[string]string{"operation": "finish"})
		}
		if err != nil {
			s.logAudit(ctx, "auth.mail_delivery_pending", map[string]string{"operation": "send"})
			return
		}
	}
}

func (s *Service) RunMailOutbox(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		s.DeliverPendingMail(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.mailWake:
		}
	}
}

func (s *Service) wakeMailOutbox() {
	select {
	case s.mailWake <- struct{}{}:
	default:
	}
}
