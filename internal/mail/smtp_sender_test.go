package mail

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestSMTPCancellationClosesSlowGreeting(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- NewSMTPSender(host, port, "", "", "").Send(ctx, Message{To: "synthetic@example.invalid"})
	}()
	select {
	case conn := <-accepted:
		defer conn.Close()
	case <-time.After(5 * time.Second):
		t.Fatal("SMTP did not connect")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled SMTP succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SMTP ignored cancellation")
	}
}
