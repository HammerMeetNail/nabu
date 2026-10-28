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

// Service delivers Web Push messages to the PWA. It is a Channel.
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

type webTarget struct{ sub Subscription }

func (t webTarget) Key() string { return "web:" + t.sub.RegistrationID }

// Targets lists the user's browser subscriptions. Without VAPID keys push is
// disabled and there are none.
func (s *Service) Targets(ctx context.Context, userID int64) ([]Target, error) {
	if s.signer == nil {
		return nil, nil
	}
	subs, err := s.store.GetSubscriptions(ctx, userID)
	if err != nil {
		log.Printf("push: get subscriptions for user %d error_class=%s", userID, diagnostics.ErrorClass(err))
		return nil, err
	}
	targets := make([]Target, 0, len(subs))
	for _, sub := range subs {
		targets = append(targets, webTarget{sub})
	}
	return targets, nil
}

// Deliver sends the message to one browser. Extra data fields (choreId,
// type, …) ride alongside the title and body so the service worker can add
// action buttons and deep-link.
func (s *Service) Deliver(ctx context.Context, target Target, m Message) Outcome {
	t, ok := target.(webTarget)
	if !ok || s.signer == nil {
		return GiveUp
	}
	sub := t.sub
	// Send-time guard: rows written before endpoint validation existed (or
	// tampered rows) must never be POSTed to. Checks are the same as
	// subscribe-time; log host only, never the capability URL.
	if !EndpointAllowed(sub.Endpoint) {
		log.Printf("push: skip disallowed endpoint for user %d host=%s", m.UserID, endpointHost(sub.Endpoint))
		return GiveUp
	}
	fields := map[string]any{"title": m.Title, "body": m.Body}
	for k, v := range m.Data {
		if k == "title" || k == "body" {
			continue
		}
		fields[k] = v
	}
	fields["userId"] = m.UserID
	fields["bindingId"] = sub.BindingID
	payload, err := json.Marshal(fields)
	if err != nil {
		return GiveUp
	}

	deliveryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	result, entered := sendRetry, false
	err = s.store.WithSubscription(deliveryCtx, m.UserID, sub, func() error {
		entered = true
		result = s.send(deliveryCtx, sub, payload)
		return nil
	})
	cancel()
	if !entered {
		if revoked(err) {
			return GiveUp
		}
		log.Printf("push: delivery user=%d error_class=%s", m.UserID, diagnostics.ErrorClass(err))
		return Retry
	}
	if result == sendStale {
		_ = s.store.DeleteSubscriptionIfCurrent(ctx, m.UserID, sub)
	}
	return result.outcome()
}

type sendResult int

const (
	sendDelivered sendResult = iota
	sendStale
	sendRefused
	sendRetry
)

func (r sendResult) outcome() Outcome {
	switch r {
	case sendDelivered:
		return Delivered
	case sendRetry:
		return Retry
	default:
		return GiveUp
	}
}

// send is called while the exact registration and its session are guarded.
// Cleanup happens after those locks are released.
func (s *Service) send(ctx context.Context, sub Subscription, payload []byte) sendResult {
	encrypted, err := EncryptPayload(payload, sub.P256DH, sub.Auth)
	if err != nil {
		log.Printf("push: encrypt error_class=%s", diagnostics.ErrorClass(err))
		return sendRefused
	}
	jwt, err := s.signer.SignJWT(sub.Endpoint)
	if err != nil {
		log.Printf("push: sign error_class=%s", diagnostics.ErrorClass(err))
		return sendRefused
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(encrypted))
	if err != nil {
		return sendRefused
	}
	req.Header.Set("Authorization", fmt.Sprintf("vapid t=%s, k=%s", jwt, s.signer.PublicKeyBase64()))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", "60")
	resp, err := s.client.Do(req)
	if err != nil {
		log.Printf("push: provider host=%s error_class=%s", endpointHost(sub.Endpoint), diagnostics.ErrorClass(err))
		return sendRetry
	}
	_ = resp.Body.Close()
	log.Printf("push: provider host=%s status=%d", endpointHost(sub.Endpoint), resp.StatusCode)
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return sendStale
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return sendRetry
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return sendDelivered
	default:
		return sendRefused
	}
}
