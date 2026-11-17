package web

import (
	"context"
	"fmt"
	"net/mail"
	"strings"

	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/email"
)

// Mailer queues an email. *email.Queue implements it; tests use a fake.
type Mailer interface {
	Enqueue(m email.Message)
}

const mailFooter = "\n-- \nprint-dis, %s\n"

func (s *Server) requestURL(id int64) string {
	return s.Config.BaseURL.String() + requestPath(id)
}

func address(u *core.User) mail.Address {
	return mail.Address{Name: u.Name, Address: *u.Email}
}

// notifyStatus emails the requester, when core.EmailsRequester says to,
// about actor moving req (as it was before the move) to `to`. It runs after
// the change is saved, and a failure here only logs.
func (s *Server) notifyStatus(ctx context.Context, actor *core.User, req *core.Request, to core.Status) {
	if s.Mailer == nil {
		return
	}
	requester, err := s.Store.GetUser(ctx, req.RequesterID)
	if err != nil {
		s.Log.Error("loading requester for email", "request", req.ID, "err", err)
		return
	}
	if !core.EmailsRequester(actor, requester, req.Status, to) {
		return
	}
	var subject, body string
	switch to {
	case core.StatusReady:
		subject = "Ready for pickup: " + req.Title
		body = fmt.Sprintf("Hi %s,\n\n%q is printed and ready for pickup.\n\n%s\n", requester.Name, req.Title, s.requestURL(req.ID))
	case core.StatusNeedsInfo:
		subject = "Question about " + req.Title
		body = fmt.Sprintf("Hi %s,\n\n%s needs a little more information before printing %q. Have a look at the request and reply there:\n\n%s\n",
			requester.Name, actor.Name, req.Title, s.requestURL(req.ID))
	}
	s.Mailer.Enqueue(email.Message{
		To:      address(requester),
		Subject: subject,
		Body:    body + fmt.Sprintf(mailFooter, "because you asked for this print."),
	})
}

// notifyNew emails the admins about requests filer has just filed from one
// form (several when it was split by color).
func (s *Server) notifyNew(ctx context.Context, filer *core.User, reqs []*core.Request) {
	if s.Mailer == nil || len(reqs) == 0 {
		return
	}
	users, err := s.Store.ListUsers(ctx)
	if err != nil {
		s.Log.Error("listing admins for email", "err", err)
		return
	}
	to := core.NewRequestRecipients(filer, users)
	if len(to) == 0 {
		return
	}
	first := reqs[0]
	who := filer.Name
	if first.RequesterID != filer.ID {
		if r, err := s.Store.GetUser(ctx, first.RequesterID); err == nil {
			who = fmt.Sprintf("%s, for %s,", filer.Name, r.Name)
		}
	}
	subject := "New request: " + first.Title
	var b strings.Builder
	if len(reqs) == 1 {
		fmt.Fprintf(&b, "%s filed a new request: %q, quantity %d.\n\n%s\n", who, first.Title, first.Quantity, s.requestURL(first.ID))
	} else {
		subject = fmt.Sprintf("%d new requests: %s", len(reqs), first.Title)
		fmt.Fprintf(&b, "%s filed %d new requests:\n\n", who, len(reqs))
		for _, r := range reqs {
			fmt.Fprintf(&b, "- %q, quantity %d: %s\n", r.Title, r.Quantity, s.requestURL(r.ID))
		}
	}
	for _, u := range to {
		s.Mailer.Enqueue(email.Message{
			To:      address(&u),
			Subject: subject,
			Body:    b.String() + fmt.Sprintf(mailFooter, "because you are an admin."),
		})
	}
}
