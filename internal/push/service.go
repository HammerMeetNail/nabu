package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/HammerMeetNail/nabu/internal/diagnostics"
	"log"
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
	return s.SendPushToUserWithData(ctx, userID, title, body, nil)
}

// SendPushToUserWithData is like SendPushToUser but merges extra fields (e.g.
// choreId, type) into the notification payload so the service worker can add
// action buttons and deep-link. Unknown to the shared PushSender interface;
// callers type-assert for it.
func (s *Service) SendPushToUserWithData(ctx context.Context, userID int64, title, body string, data map[string]any) error {
	if s.signer == nil {
		return nil // push disabled (no VAPID keys configured)
	}
	subs, err := s.store.GetSubscriptions(ctx, userID)
	if err != nil {
		log.Printf("push: get subscriptions for user %d error_class=%s", userID, diagnostics.ErrorClass(err))
		return err
	}
	if len(subs) == 0 {
		log.Printf("push: no subscriptions for user %d", userID)
		return nil
	}

	fields := map[string]any{"title": title, "body": body}
	for k, v := range data {
		if k == "title" || k == "body" {
			continue
		}
		fields[k] = v
	}
	for _, sub := range subs {
		if err := ctx.Err(); err != nil {
			return err
		}
		fields["userId"] = userID
		fields["bindingId"] = sub.BindingID
		payload, err := json.Marshal(fields)
		if err != nil {
			return err
		}
		// Send-time guard: rows written before endpoint validation existed
		// (or tampered rows) must never be POSTed to. Checks are the same as
		// subscribe-time; log host only, never the capability URL.
		if !EndpointAllowed(sub.Endpoint) {
			log.Printf("push: skip disallowed endpoint for user %d host=%s", userID, endpointHost(sub.Endpoint))
			continue
		}

		deliveryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		stale := false
		err = s.store.WithSubscription(deliveryCtx, userID, sub, func() error {
			var err error
			stale, err = s.send(deliveryCtx, sub, payload)
			return err
		})
		cancel()
		if err != nil {
			log.Printf("push: delivery user=%d error_class=%s", userID, diagnostics.ErrorClass(err))
		}
		if stale {
			_ = s.store.DeleteSubscriptionIfCurrent(ctx, userID, sub)
		}
	}

	return nil
}

// send is called while the exact registration and its session are guarded.
// Cleanup happens after those locks are released.
func (s *Service) send(ctx context.Context, sub Subscription, payload []byte) (bool, error) {
	encrypted, err := EncryptPayload(payload, sub.P256DH, sub.Auth)
	if err != nil {
		return false, err
	}
	jwt, err := s.signer.SignJWT(sub.Endpoint)
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(encrypted))
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("vapid t=%s, k=%s", jwt, s.signer.PublicKeyBase64()))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", "60")
	resp, err := s.client.Do(req)
	if err != nil {
		return false, err
	}
	_ = resp.Body.Close()
	log.Printf("push: provider host=%s status=%d", endpointHost(sub.Endpoint), resp.StatusCode)
	return resp.StatusCode == 404 || resp.StatusCode == 410, nil
}
