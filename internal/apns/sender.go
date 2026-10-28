package apns

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/HammerMeetNail/nabu/internal/diagnostics"
	"github.com/HammerMeetNail/nabu/internal/push"
	"io"
	"log"
	"net/http"
	"time"
)

const (
	hostSandbox    = "https://api.sandbox.push.apple.com"
	hostProduction = "https://api.push.apple.com"
)

// Client sends alert pushes to a user's registered devices. It is a
// push.Channel, delivering the same jobs as the Web Push service.
type Client struct {
	store  Store
	signer *ProviderTokenSigner
	// topic is the apns-topic header — the app's bundle ID.
	topic      string
	httpClient *http.Client

	// hostOverride redirects both environments to one base URL; used by
	// tests to point the client at a fake APNs server.
	hostOverride string
}

func NewClient(store Store, signer *ProviderTokenSigner, topic string) *Client {
	// Go's http.Client negotiates HTTP/2 for https endpoints automatically,
	// which is all APNs requires.
	return &Client{
		store:      store,
		signer:     signer,
		topic:      topic,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) host(environment string) string {
	if c.hostOverride != "" {
		return c.hostOverride
	}
	if environment == EnvironmentSandbox {
		return hostSandbox
	}
	return hostProduction
}

type deviceTarget struct{ device Device }

func (t deviceTarget) Key() string { return "apns:" + t.device.RegistrationID }

// Targets lists the devices the user has registered. Without APNs settings
// there are none.
func (c *Client) Targets(ctx context.Context, userID int64) ([]push.Target, error) {
	if c == nil || c.signer == nil {
		return nil, nil
	}
	devices, err := c.store.DevicesForUser(ctx, userID)
	if err != nil {
		log.Printf("apns: devices user=%d error_class=%s", userID, diagnostics.ErrorClass(err))
		return nil, err
	}
	targets := make([]push.Target, 0, len(devices))
	for _, device := range devices {
		targets = append(targets, deviceTarget{device})
	}
	return targets, nil
}

// Deliver sends an alert push to one device. Extra data fields (choreId,
// type, …) ride alongside the aps dictionary for the client's notification
// handling; a "category" field maps to aps.category so action buttons can
// attach. A terminally invalid token is pruned.
func (c *Client) Deliver(ctx context.Context, target push.Target, m push.Message) push.Outcome {
	t, ok := target.(deviceTarget)
	if !ok || c == nil || c.signer == nil {
		return push.GiveUp
	}
	device := t.device
	fields := make(map[string]any, len(m.Data)+1)
	for k, v := range m.Data {
		fields[k] = v
	}
	fields["userId"] = m.UserID
	payload, err := c.buildPayload(m.Title, m.Body, fields)
	if err != nil {
		return push.GiveUp
	}

	deliveryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	result, entered := sendRetry, false
	err = c.store.WithDevice(deliveryCtx, device, func() error {
		entered = true
		result = c.sendToDevice(deliveryCtx, device, payload, m.CollapseID)
		return nil
	})
	cancel()
	if !entered {
		if revoked(err) {
			return push.GiveUp
		}
		log.Printf("apns: delivery user=%d error_class=%s", m.UserID, diagnostics.ErrorClass(err))
		return push.Retry
	}
	switch result {
	case sendDelivered:
		return push.Delivered
	case sendRetry:
		return push.Retry
	case sendStale:
		_ = c.store.DeleteDeviceIfCurrent(ctx, device)
	}
	return push.GiveUp
}

func (c *Client) buildPayload(title, body string, data map[string]any) ([]byte, error) {
	aps := map[string]any{
		"alert": map[string]string{"title": "Nabu", "body": "Open Nabu to check your household updates."},
		"sound": "default",
	}
	fields := map[string]any{}
	for k, v := range data {
		switch k {
		case "title", "body", "aps":
			continue
		case "category":
			if s, ok := v.(string); ok && s != "" {
				aps["category"] = s
			}
		default:
			fields[k] = v
		}
	}
	fields["aps"] = aps
	return json.Marshal(fields)
}

type sendResult int

const (
	sendDelivered sendResult = iota
	sendStale
	sendRefused
	sendRetry
)

func (c *Client) sendToDevice(ctx context.Context, device Device, payload []byte, collapseID string) sendResult {
	token, err := c.signer.Token()
	if err != nil {
		log.Printf("apns: provider token error_class=%s", diagnostics.ErrorClass(err))
		return sendRefused
	}

	url := c.host(device.Environment) + "/3/device/" + device.Token
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		log.Printf("apns: create request error_class=%s", diagnostics.ErrorClass(err))
		return sendRefused
	}
	req.Header.Set("Authorization", "bearer "+token)
	req.Header.Set("apns-topic", c.topic)
	req.Header.Set("apns-push-type", "alert")
	req.Header.Set("apns-priority", "10")
	req.Header.Set("Content-Type", "application/json")
	// A delivery retried after a lost response replaces the first on the
	// device instead of showing twice.
	if collapseID != "" {
		req.Header.Set("apns-collapse-id", collapseID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("apns: send user=%d error_class=%s", device.UserID, diagnostics.ErrorClass(err))
		return sendRetry
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		log.Printf("apns: sent user=%d status=200", device.UserID)
		return sendDelivered
	}

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var apnsErr struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(respBody, &apnsErr)
	log.Printf("apns: send user=%d status=%d", device.UserID, resp.StatusCode)

	// 410 Unregistered means the token is gone for good; BadDeviceToken means
	// it was never valid for this environment. Both are terminal — prune.
	switch {
	case resp.StatusCode == http.StatusGone || apnsErr.Reason == "BadDeviceToken" || apnsErr.Reason == "Unregistered":
		return sendStale
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return sendRetry
	default:
		return sendRefused
	}
}
