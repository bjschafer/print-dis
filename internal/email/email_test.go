package email

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRelay is just enough of an SMTP server to accept one message per connection.
type fakeRelay struct {
	ln   net.Listener
	mu   sync.Mutex
	got  []received
	done chan struct{}
}

type received struct {
	from, to string
	data     string
}

func newFakeRelay(t *testing.T) *fakeRelay {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeRelay{ln: ln, done: make(chan struct{}, 10)}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func (f *fakeRelay) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	r := bufio.NewReader(c)
	say := func(s string) { _, _ = io.WriteString(c, s+"\r\n") }
	say("220 fake ESMTP")
	var msg received
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		cmd := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			say("250 fake")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			msg.from = strings.Trim(line[len("MAIL FROM:"):], "<> ")
			say("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			msg.to = strings.Trim(line[len("RCPT TO:"):], "<> ")
			say("250 ok")
		case cmd == "DATA":
			say("354 go ahead")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			msg.data = b.String()
			f.mu.Lock()
			f.got = append(f.got, msg)
			f.mu.Unlock()
			f.done <- struct{}{}
			say("250 queued")
		case cmd == "QUIT":
			say("221 bye")
			return
		default:
			say("502 no")
		}
	}
}

var from = mail.Address{Name: "print-dis", Address: "prints@example.com"}

func TestSMTPSend(t *testing.T) {
	f := newFakeRelay(t)
	s := &SMTP{Addr: f.ln.Addr().String(), From: from, Timeout: 5 * time.Second}
	m := Message{
		To:      mail.Address{Name: "Phii", Address: "phii@example.com"},
		Subject: "Ready for pickup: Connector Corner\r\nBcc: evil@example.com",
		Body:    "Hi Phii,\n\n.leading dot and café\n",
	}
	if err := s.Send(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	<-f.done
	f.mu.Lock()
	got := f.got[0]
	f.mu.Unlock()
	if got.from != "prints@example.com" || got.to != "phii@example.com" {
		t.Fatalf("envelope = %q -> %q", got.from, got.to)
	}
	head, body, ok := strings.Cut(got.data, "\r\n\r\n")
	if !ok {
		t.Fatalf("no header/body split in %q", got.data)
	}
	if strings.Contains(head, "\r\nBcc:") {
		t.Errorf("subject injected a header:\n%s", head)
	}
	for _, want := range []string{`From: "print-dis" <prints@example.com>`, `To: "Phii" <phii@example.com>`, "Subject: Ready for pickup: Connector Corner Bcc: evil@example.com", "Auto-Submitted: auto-generated"} {
		if !strings.Contains(head, want) {
			t.Errorf("header missing %q:\n%s", want, head)
		}
	}
	// The relay sees the dot doubled; undo it, then quoted-printable.
	body = strings.ReplaceAll(body, "\r\n..", "\r\n.")
	plain, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(body)))
	if err != nil {
		t.Fatal(err)
	}
	if want := "Hi Phii,\r\n\r\n.leading dot and café\r\n"; !strings.HasPrefix(string(plain), want) {
		t.Errorf("body = %q, want prefix %q", plain, want)
	}
}

func TestSMTPRelayDown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	s := &SMTP{Addr: addr, From: from, Timeout: 2 * time.Second}
	if err := s.Send(context.Background(), Message{To: mail.Address{Address: "a@example.com"}}); err == nil {
		t.Fatal("send to a closed port succeeded")
	}
}

func TestSMTPRelayHangs(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err == nil {
			defer func() { _ = c.Close() }()
			time.Sleep(3 * time.Second) // never says hello
		}
	}()
	s := &SMTP{Addr: ln.Addr().String(), From: from, Timeout: 200 * time.Millisecond}
	start := time.Now()
	if err := s.Send(context.Background(), Message{To: mail.Address{Address: "a@example.com"}}); err == nil {
		t.Fatal("send to a silent server succeeded")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("took %s; the timeout was not applied", d)
	}
}

type flaky struct {
	mu    sync.Mutex
	fails int
	calls int
	sent  chan Message
}

func (f *flaky) Send(_ context.Context, m Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls <= f.fails {
		return errors.New("relay down")
	}
	f.sent <- m
	return nil
}

func TestQueueRetries(t *testing.T) {
	f := &flaky{fails: 2, sent: make(chan Message, 1)}
	q := NewQueue(f, slog.New(slog.DiscardHandler))
	q.Backoff = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	go q.Run(t.Context())
	q.Enqueue(Message{To: mail.Address{Address: "a@example.com"}, Subject: "hi"})
	select {
	case m := <-f.sent:
		if m.Subject != "hi" {
			t.Fatalf("sent %+v", m)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("never sent")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls != 3 {
		t.Fatalf("calls = %d, want 3", f.calls)
	}
}

func TestQueueGivesUp(t *testing.T) {
	f := &flaky{fails: 100, sent: make(chan Message, 1)}
	q := NewQueue(f, slog.New(slog.DiscardHandler))
	q.Backoff = []time.Duration{time.Millisecond}
	q.deliver(context.Background(), Message{To: mail.Address{Address: "a@example.com"}})
	if f.calls != 2 {
		t.Fatalf("calls = %d, want 2", f.calls)
	}
}

func TestEnqueueNeverBlocks(t *testing.T) {
	q := NewQueue(&flaky{}, slog.New(slog.DiscardHandler))
	done := make(chan struct{})
	go func() {
		for range cap(q.ch) + 5 { // nothing is running the queue
			q.Enqueue(Message{})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Enqueue blocked on a full queue")
	}
}
