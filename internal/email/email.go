// Package email sends plain-text notifications through an SMTP relay.
//
// Sending never blocks a request: handlers put messages on a Queue, and
// one goroutine delivers them, retrying a few times. A message that still
// cannot be sent is logged and dropped.
package email

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// Message is one email to one person.
type Message struct {
	To      mail.Address
	Subject string
	Body    string // plain text, "\n" line endings
}

// Sender delivers one message.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// SMTP sends through a relay that needs neither authentication nor TLS,
// such as the in-cluster Postfix.
type SMTP struct {
	Addr     string       // host:port
	From     mail.Address // envelope sender and From header
	Hostname string       // sent with EHLO; "localhost" when empty
	Timeout  time.Duration
	Now      func() time.Time
}

// Send delivers m, giving up after s.Timeout (30s when zero).
func (s *SMTP) Send(ctx context.Context, m Message) error {
	timeout := s.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", s.Addr)
	if err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return err
	}
	host, _, _ := net.SplitHostPort(s.Addr)
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = c.Close() }()
	hello := s.Hostname
	if hello == "" {
		hello = "localhost"
	}
	if err := c.Hello(hello); err != nil {
		return err
	}
	if err := c.Mail(s.From.Address); err != nil {
		return err
	}
	if err := c.Rcpt(m.To.Address); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(Render(s.From, m, now())); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// Render builds the message as sent: headers, then a quoted-printable body.
func Render(from mail.Address, m Message, date time.Time) []byte {
	var b bytes.Buffer
	header := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	header("From", from.String())
	header("To", m.To.String())
	header("Subject", mime.QEncoding.Encode("utf-8", oneLine(m.Subject)))
	header("Date", date.Format(time.RFC1123Z))
	header("MIME-Version", "1.0")
	header("Content-Type", `text/plain; charset="utf-8"`)
	header("Content-Transfer-Encoding", "quoted-printable")
	header("Auto-Submitted", "auto-generated")
	b.WriteString("\r\n")
	qp := quotedprintable.NewWriter(&b)
	_, _ = qp.Write([]byte(strings.ReplaceAll(m.Body, "\n", "\r\n")))
	_ = qp.Close()
	b.WriteString("\r\n")
	return b.Bytes()
}

// oneLine keeps a header value on one line, so a title cannot add headers.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Queue delivers messages in the background.
type Queue struct {
	sender Sender
	log    *slog.Logger
	ch     chan Message
	// Backoff is the wait before each retry; its length is the number of retries.
	Backoff []time.Duration
}

// NewQueue holds up to 100 waiting messages.
func NewQueue(sender Sender, log *slog.Logger) *Queue {
	if log == nil {
		log = slog.Default()
	}
	return &Queue{
		sender:  sender,
		log:     log,
		ch:      make(chan Message, 100),
		Backoff: []time.Duration{10 * time.Second, time.Minute, 5 * time.Minute},
	}
}

// Enqueue never blocks: when the queue is full the message is dropped.
func (q *Queue) Enqueue(m Message) {
	select {
	case q.ch <- m:
	default:
		q.log.Error("email queue full; dropping message", "to", m.To.Address, "subject", m.Subject)
	}
}

// Run delivers until ctx is done. Messages still queued then are lost.
func (q *Queue) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			if n := len(q.ch); n > 0 {
				q.log.Warn("shutting down with unsent email", "count", n)
			}
			return
		case m := <-q.ch:
			q.deliver(ctx, m)
		}
	}
}

func (q *Queue) deliver(ctx context.Context, m Message) {
	for attempt := 0; ; attempt++ {
		err := q.sender.Send(ctx, m)
		if err == nil {
			q.log.Info("sent email", "to", m.To.Address, "subject", m.Subject)
			return
		}
		if attempt >= len(q.Backoff) || errors.Is(err, context.Canceled) {
			q.log.Error("sending email failed; giving up", "to", m.To.Address, "subject", m.Subject, "attempts", attempt+1, "err", err)
			return
		}
		q.log.Warn("sending email failed; will retry", "to", m.To.Address, "attempt", attempt+1, "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(q.Backoff[attempt]):
		}
	}
}
