package mail

import "context"

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
	Messages []Message
}

func NewMemorySender() *MemorySender {
	return &MemorySender{Messages: []Message{}}
}

func (s *MemorySender) Send(_ context.Context, msg Message) error {
	s.Messages = append(s.Messages, msg)
	return nil
}

type LogSender struct{}

func (LogSender) Send(_ context.Context, msg Message) error {
	return nil
}

type UnavailableSender struct{}

func (UnavailableSender) Send(_ context.Context, _ Message) error {
	return nil
}
