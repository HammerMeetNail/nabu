package mail

import (
	"context"
	"net"
	netmail "net/mail"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

type sentMail struct {
	envelope string
	data     string
}

// fakeSMTP accepts one connection and reports the message's envelope sender
// and data once the client quits.
func fakeSMTP(t *testing.T) (host, port string, sent <-chan sentMail) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	ch := make(chan sentMail, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		tp := textproto.NewConn(conn)
		var mail sentMail
		_ = tp.PrintfLine("220 fake ESMTP")
		for {
			line, err := tp.ReadLine()
			if err != nil {
				return
			}
			upper := strings.ToUpper(line)
			switch {
			case strings.HasPrefix(upper, "MAIL FROM:"):
				mail.envelope = line[len("MAIL FROM:"):]
				_ = tp.PrintfLine("250 ok")
			case upper == "DATA":
				_ = tp.PrintfLine("354 go ahead")
				data, err := tp.ReadDotBytes()
				if err != nil {
					return
				}
				mail.data = string(data)
				_ = tp.PrintfLine("250 queued")
			case upper == "QUIT":
				_ = tp.PrintfLine("221 bye")
				ch <- mail
				return
			default:
				_ = tp.PrintfLine("250 ok")
			}
		}
	}()
	host, port, _ = net.SplitHostPort(listener.Addr().String())
	return host, port, ch
}

func TestSMTPSenderKeepsTheDisplayNameOutOfTheEnvelope(t *testing.T) {
	for _, tc := range []struct{ from, envelope, header string }{
		{"Nabu <noreply@nabu-app.com>", "<noreply@nabu-app.com>", `"Nabu" <noreply@nabu-app.com>`},
		{"noreply@nabu-app.com", "<noreply@nabu-app.com>", "noreply@nabu-app.com"},
		{"", "<no-reply@nabu.local>", "no-reply@nabu.local"},
	} {
		t.Run(tc.from, func(t *testing.T) {
			host, port, sent := fakeSMTP(t)
			err := NewSMTPSender(host, port, "", "", tc.from).Send(context.Background(),
				Message{To: "someone@example.invalid", Subject: "Verify your Nabu email", Body: "<p>hi</p>"})
			if err != nil {
				t.Fatal(err)
			}
			var mail sentMail
			select {
			case mail = <-sent:
			case <-time.After(5 * time.Second):
				t.Fatal("the fake server received no message")
			}
			if mail.envelope != tc.envelope {
				t.Fatalf("MAIL FROM = %q, want %q", mail.envelope, tc.envelope)
			}
			msg, err := netmail.ReadMessage(strings.NewReader(mail.data))
			if err != nil {
				t.Fatal(err)
			}
			if got := msg.Header.Get("From"); got != tc.header {
				t.Fatalf("From header = %q, want %q", got, tc.header)
			}
		})
	}
}

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
