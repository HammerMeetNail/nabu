package mail

import (
	"context"
	"errors"
	netmail "net/mail"
	"slices"
	"strings"
	"sync"
)

type Message struct {
	To      string
	Subject string
	Body    string
}

type Sender interface {
	Send(ctx context.Context, msg Message) error
}

type NopSender struct{}

func (NopSender) Send(_ context.Context, _ Message) error { return nil }

type MemorySender struct {
	mu       sync.Mutex
	messages []Message
}

func NewMemorySender() *MemorySender {
	return &MemorySender{}
}

func (s *MemorySender) Send(_ context.Context, msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
	return nil
}

func (s *MemorySender) Messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.messages)
}

func (s *MemorySender) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = nil
}

type LogSender struct{}

func (LogSender) Send(_ context.Context, msg Message) error {
	return nil
}

type UnavailableSender struct{}

func (UnavailableSender) Send(_ context.Context, _ Message) error {
	return errors.New("mail is not configured")
}

// SkipInvalid drops mail to an address in the reserved .invalid domain
// (RFC 2606), which no server can deliver: sending it would only spend the
// mail provider's quota and earn a bounce. The production smoke test signs up
// with such an address, so it sends no email.
func SkipInvalid(next Sender) Sender { return skipInvalid{next: next} }

type skipInvalid struct{ next Sender }

func (s skipInvalid) Send(ctx context.Context, msg Message) error {
	if Undeliverable(msg.To) {
		return nil
	}
	return s.next.Send(ctx, msg)
}

// Undeliverable reports whether to is an address in the .invalid domain.
func Undeliverable(to string) bool {
	addr := to
	if parsed, err := netmail.ParseAddress(to); err == nil {
		addr = parsed.Address
	}
	at := strings.LastIndexByte(addr, '@')
	if at < 0 {
		return false
	}
	domain := strings.TrimSuffix(strings.ToLower(addr[at+1:]), ".")
	return domain == "invalid" || strings.HasSuffix(domain, ".invalid")
}
