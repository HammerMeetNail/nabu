package mail

import (
	"context"
	"errors"
	"slices"
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
