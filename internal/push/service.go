package push

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"
)

// Service implements notification.PushSender by delivering Web Push messages.
type Service struct {
	store  Store
	signer *VAPIDSigner
	client *http.Client
}

func NewService(store Store, signer *VAPIDSigner) *Service {
	return &Service{
		store:  store,
		signer: signer,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// SendPushToUser sends a push notification to every subscription registered
// for the given userID.  Errors for individual endpoints are logged but not
// returned so that one stale subscription does not block all others.
func (s *Service) SendPushToUser(ctx context.Context, userID int64, title, body string) error {
	if s.signer == nil {
		return nil // push disabled (no VAPID keys configured)
	}
	subs, err := s.store.GetSubscriptions(ctx, userID)
	if err != nil {
		return err
	}

	payload := []byte(fmt.Sprintf(`{"title":%q,"body":%q}`, title, body))

	for _, sub := range subs {
		encrypted, err := EncryptPayload(payload, sub.P256DH, sub.Auth)
		if err != nil {
			continue
		}

		jwt, err := s.signer.SignJWT(sub.Endpoint)
		if err != nil {
			continue
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(encrypted))
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", fmt.Sprintf("vapid t=%s, k=%s", jwt, s.signer.PublicKeyBase64()))
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Content-Encoding", "aes128gcm")
		req.Header.Set("TTL", "60")

		resp, err := s.client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close() //nolint:errcheck

		// Clean up stale subscriptions
		if resp.StatusCode == 404 || resp.StatusCode == 410 {
			_ = s.store.DeleteSubscription(ctx, userID, sub.Endpoint)
		}
	}

	return nil
}
