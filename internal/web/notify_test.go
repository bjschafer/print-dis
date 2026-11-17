package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/email"
)

type fakeMailer struct {
	mu   sync.Mutex
	sent []email.Message
}

func (f *fakeMailer) Enqueue(m email.Message) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
}

// take returns what was sent since the last call.
func (f *fakeMailer) take() []email.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.sent
	f.sent = nil
	return out
}

func newMailHarness(t *testing.T) (*harness, *fakeMailer) {
	t.Helper()
	m := &fakeMailer{}
	return newHarnessDeps(t, func(d *Deps) { d.Mailer = m }), m
}

func TestStatusEmails(t *testing.T) {
	h, m := newMailHarness(t)
	ctx := context.Background()
	owner := h.owner()
	alice := h.friend("Alice")
	file := func(title string, by *core.User) int64 {
		r := &core.Request{Title: title, Status: core.StatusPrinting, RequesterID: by.ID, CreatedByID: owner.ID, Quantity: 1, Timing: core.TimingWhenever}
		if err := h.store.CreateRequest(ctx, r); err != nil {
			t.Fatal(err)
		}
		return r.ID
	}
	one := func(what string) email.Message {
		t.Helper()
		got := m.take()
		if len(got) != 1 {
			t.Fatalf("%s: sent %d emails, want 1: %+v", what, len(got), got)
		}
		return got[0]
	}
	none := func(what string) {
		t.Helper()
		if got := m.take(); len(got) != 0 {
			t.Fatalf("%s: sent %+v, want none", what, got)
		}
	}

	// The status button.
	corner := file("Connector Corner", alice)
	if code, _, _ := h.post("/requests/"+itoa(corner)+"/status", url.Values{"status": {"ready"}}); code != http.StatusSeeOther {
		t.Fatalf("set ready: %d", code)
	}
	got := one("ready")
	if got.To.Address != "alice@example.com" || got.To.Name != "Alice" || got.Subject != "Ready for pickup: Connector Corner" {
		t.Errorf("ready email: %+v", got)
	}
	if want := h.s.Config.BaseURL.String() + "/requests/" + itoa(corner); !strings.Contains(got.Body, want) || !strings.Contains(got.Body, "Hi Alice") {
		t.Errorf("ready body missing link or greeting:\n%s", got.Body)
	}
	if code, _, _ := h.post("/requests/"+itoa(corner)+"/status", url.Values{"status": {"delivered"}}); code != http.StatusSeeOther {
		t.Fatalf("deliver: %d", code)
	}
	none("delivered")

	// Dragging on the board.
	bracket := file("Bracket", alice)
	if code, _, _ := h.post("/requests/"+itoa(bracket)+"/move", url.Values{"status": {"needs_info"}, "order": {itoa(bracket)}}, "X-CSRF-Token", h.csrf); code != http.StatusNoContent {
		t.Fatalf("drag: %d", code)
	}
	if got := one("needs info"); got.Subject != "Question about Bracket" || !strings.Contains(got.Body, "more information") {
		t.Errorf("needs info email: %+v", got)
	}
	if code, _, _ := h.post("/requests/"+itoa(bracket)+"/move", url.Values{"status": {"printing"}, "order": {itoa(bracket)}}, "X-CSRF-Token", h.csrf); code != http.StatusNoContent {
		t.Fatalf("drag: %d", code)
	}
	none("printing")

	// The API.
	clip := file("Clip", alice)
	if code, _, raw := h.api("POST", "/api/v1/requests/"+itoa(clip)+"/status", h.token(owner), map[string]string{"status": "ready"}, nil); code != http.StatusOK {
		t.Fatalf("api ready: %d %s", code, raw)
	}
	one("api ready")

	// The owner's own request is delivered, not readied, and nobody is told.
	mine := file("Mine", owner)
	if code, _, _ := h.post("/requests/"+itoa(mine)+"/status", url.Values{"status": {"ready"}}); code != http.StatusSeeOther {
		t.Fatalf("own ready: %d", code)
	}
	none("own request")

	// Someone with no email address gets nothing, and nothing breaks.
	ghost, err := h.store.CreatePlaceholder(ctx, "Ghost", "")
	if err != nil {
		t.Fatal(err)
	}
	if code, _, _ := h.post("/requests/"+itoa(file("Ghost's", ghost))+"/status", url.Values{"status": {"ready"}}); code != http.StatusSeeOther {
		t.Fatalf("placeholder ready: %d", code)
	}
	none("no email")
}

func TestNewRequestEmails(t *testing.T) {
	h, m := newMailHarness(t)
	owner := h.owner()
	alice := h.as(h.friend("Alice"))

	code, loc, body := alice.post("/requests", url.Values{"title": {"Cable clip"}, "quantity": {"3"}, "timing": {"whenever"}})
	if code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", code, body)
	}
	got := m.take()
	if len(got) != 1 || got[0].To.Address != *owner.Email || got[0].Subject != "New request: Cable clip" {
		t.Fatalf("sent %+v", got)
	}
	if !strings.Contains(got[0].Body, "Alice filed a new request") || !strings.Contains(got[0].Body, h.s.Config.BaseURL.String()+loc) {
		t.Errorf("body:\n%s", got[0].Body)
	}

	// The owner filing is not news to the owner.
	if code, _, body := h.post("/requests", url.Values{"title": {"Mine"}, "quantity": {"1"}, "timing": {"whenever"}}); code != http.StatusSeeOther {
		t.Fatalf("owner create: %d %s", code, body)
	}
	if got := m.take(); len(got) != 0 {
		t.Fatalf("owner's own request emailed %+v", got)
	}

	// The API is for admins, so the filer is the only admin here.
	if code, _, raw := h.api("POST", "/api/v1/requests", h.token(owner), map[string]any{"title": "Hook"}, nil); code != http.StatusCreated {
		t.Fatalf("api create: %d %s", code, raw)
	}
	if got := m.take(); len(got) != 0 {
		t.Fatalf("api create sent %+v", got)
	}
}

func TestNoMailerNoEmail(t *testing.T) {
	// Without a Mailer every path still works; this is the default harness.
	h := newHarness(t)
	alice := h.friend("Alice")
	r := &core.Request{Title: "x", Status: core.StatusPrinting, RequesterID: alice.ID, CreatedByID: h.owner().ID, Quantity: 1, Timing: core.TimingWhenever}
	if err := h.store.CreateRequest(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := h.post("/requests/"+itoa(r.ID)+"/status", url.Values{"status": {"ready"}}); code != http.StatusSeeOther {
		t.Fatalf("set ready: %d", code)
	}
}
