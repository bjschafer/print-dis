package web

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/store"
)

// TestAdminFlow covers M1's "done when": create, edit, comment on, drag and
// deliver a request; printers with bed sizes; eligibility with explanations.
func TestAdminFlow(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// Printers.
	for _, p := range []url.Values{
		{"name": {"H2D"}, "bed_x": {"325"}, "bed_y": {"320"}, "bed_z": {"325"}, "multi_material": {"on"}, "enclosed": {"on"}, "active": {"on"}},
		{"name": {"Trident"}, "bed_x": {"350"}, "bed_y": {"350"}, "bed_z": {"300"}, "enclosed": {"on"}, "active": {"on"}},
	} {
		if code, loc, body := h.post("/printers", p); code != http.StatusSeeOther || loc != "/printers" {
			t.Fatalf("create printer: %d %s %s", code, loc, body)
		}
	}
	code, _, body := h.post("/printers", url.Values{"name": {"Bad"}, "bed_x": {"0"}, "bed_y": {"1"}, "bed_z": {"1"}})
	if code != http.StatusUnprocessableEntity || !strings.Contains(body, "Bed width") {
		t.Fatalf("invalid printer: %d", code)
	}
	code, _, _ = h.post("/printers", url.Values{"name": {"H2D"}, "bed_x": {"1"}, "bed_y": {"1"}, "bed_z": {"1"}})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate printer: %d", code)
	}
	_, body = h.get("/printers")
	h.mustContain(body, "H2D", "325×320×325 mm", "Trident")

	// Create.
	code, loc, body := h.post("/requests", url.Values{
		"title": {"Spool holder"}, "quantity": {"4"}, "timing": {"firm"}, "needed_by": {"2026-11-12"},
		"material": {"petg"}, "colors_form": {"1"}, "color_0": {"c:blue"}, "needs_multi_material": {"on"},
		"size_x": {"100"}, "size_y": {"80"}, "size_z": {"50"}, "source_url": {"https://www.printables.com/model/1"},
	})
	if code != http.StatusSeeOther || !strings.HasPrefix(loc, "/requests/") {
		t.Fatalf("create: %d %q %s", code, loc, body)
	}
	_, body = h.get(loc)
	h.mustContain(body, "Spool holder", "PETG", "no multi-material", "eligible")

	// Invalid create re-renders with errors.
	code, _, body = h.post("/requests", url.Values{"title": {""}, "size_x": {"10"}})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid create: %d", code)
	}
	h.mustContain(body, "Give the request a title", "all three dimensions")

	// Edit: assign an ineligible printer, which is allowed with a warning.
	reqs, _ := h.store.ListRequests(ctx, store.Filter{})
	if len(reqs) != 1 {
		t.Fatalf("got %d requests", len(reqs))
	}
	r := reqs[0]
	var trident int64
	ps, _ := h.store.ListPrinters(ctx)
	for _, p := range ps {
		if p.Name == "Trident" {
			trident = p.ID
		}
	}
	edit := url.Values{
		"title": {"Spool holder v2"}, "quantity": {"4"}, "timing": {"firm"}, "needed_by": {"2026-11-12"},
		"material": {"PETG"}, "colors_form": {"1"}, "needs_multi_material": {"on"}, "size_x": {"100"}, "size_y": {"80"}, "size_z": {"50"},
		"printer_id": {itoa(trident)}, "printed_count": {"1"}, "requester_id": {itoa(r.RequesterID)},
	}
	if code, _, body := h.post(loc, edit); code != http.StatusSeeOther {
		t.Fatalf("edit: %d %s", code, body)
	}
	_, body = h.get(loc)
	h.mustContain(body, "Spool holder v2", "Assigned to", "Trident is not eligible", "Other printers", "changed title, printed, link, color, printer")

	// Comment, both plain and via htmx.
	if code, _, _ := h.post(loc+"/comments", url.Values{"body": {"first"}}); code != http.StatusSeeOther {
		t.Fatalf("comment: %d", code)
	}
	code, _, body = h.post(loc+"/comments", url.Values{"body": {"<b>second</b>"}}, "HX-Request", "true")
	if code != http.StatusOK || !strings.Contains(body, `id="thread-events"`) || !strings.Contains(body, "&lt;b&gt;second") {
		t.Fatalf("htmx comment: %d %s", code, body)
	}

	// Board shows it.
	_, body = h.get("/")
	h.mustContain(body, "Spool holder v2", "1/4", "Trident", `data-can-move`)

	// Drag into Printing.
	code, _, _ = h.post(loc+"/move", url.Values{"status": {"printing"}, "order": {itoa(r.ID)}}, "X-CSRF-Token", h.csrf)
	if code != http.StatusNoContent {
		t.Fatalf("move: %d", code)
	}
	got, _ := h.store.GetRequest(ctx, r.ID)
	if got.Status != core.StatusPrinting {
		t.Fatalf("status after move: %s", got.Status)
	}

	// Deliver; it leaves the board but stays in the table.
	if code, _, _ := h.post(loc+"/status", url.Values{"status": {"delivered"}}); code != http.StatusSeeOther {
		t.Fatalf("deliver: %d", code)
	}
	_, body = h.get("/")
	h.mustNotContain(body, "Spool holder v2")
	_, body = h.get("/requests")
	h.mustContain(body, "Spool holder v2", "Delivered")
	_, body = h.get(loc)
	h.mustContain(body, "requested -&gt; printing", "printing -&gt; delivered")

	// Filters.
	_, body = h.get("/requests?printer=" + itoa(trident))
	h.mustContain(body, "Spool holder v2")
	_, body = h.get("/requests?material=PLA")
	h.mustContain(body, "No requests match")
}

func TestDeliverFromBoard(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	alice := h.friend("Alice")
	ids := map[string]int64{}
	for _, r := range []*core.Request{
		{Title: "Lamp shade", Status: core.StatusReady},
		{Title: "Spool holder", Status: core.StatusPrinting},
	} {
		r.RequesterID, r.CreatedByID, r.Quantity, r.Timing = alice.ID, alice.ID, 1, core.TimingWhenever
		if err := h.store.CreateRequest(ctx, r); err != nil {
			t.Fatal(err)
		}
		ids[r.Title] = r.ID
	}
	ready := "/requests/" + itoa(ids["Lamp shade"])

	// Only the Ready card offers it, and only to the admin.
	_, body := h.get("/")
	h.mustContain(body, `action="`+ready+`/status" class="inline card-deliver" data-deliver`)
	if n := strings.Count(body, "data-deliver"); n != 1 {
		t.Errorf("%d Delivered buttons, want 1", n)
	}
	ha := h.as(alice)
	_, body = ha.get("/")
	ha.mustContain(body, "Lamp shade")
	ha.mustNotContain(body, "data-deliver")

	// Script posts it and removes the card itself.
	if code, _, _ := h.post(ready+"/status", url.Values{"status": {"delivered"}, "back": {"/"}}, "HX-Request", "true"); code != http.StatusNoContent {
		t.Fatalf("deliver: %d", code)
	}
	if got, _ := h.store.GetRequest(ctx, ids["Lamp shade"]); got.Status != core.StatusDelivered {
		t.Fatalf("status: %s", got.Status)
	}
	_, body = h.get("/")
	h.mustNotContain(body, "Lamp shade")
}

func TestCSRFRequired(t *testing.T) {
	h := newHarness(t)
	h.get("/")
	code, _, _ := h.post("/printers", url.Values{"csrf": {"wrong"}, "name": {"x"}, "bed_x": {"1"}, "bed_y": {"1"}, "bed_z": {"1"}})
	if code != http.StatusForbidden {
		t.Fatalf("bad token: %d", code)
	}
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/printers", strings.NewReader("csrf="+h.csrf))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	res, err := h.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site: %d", res.StatusCode)
	}
}

func TestNewPrefillsURL(t *testing.T) {
	h := newHarness(t)
	_, body := h.get("/requests/new?url=" + url.QueryEscape("https://example.com/m/1"))
	h.mustContain(body, `value="https://example.com/m/1"`)
	_, body = h.get("/requests/new?url=" + url.QueryEscape("javascript:alert(1)"))
	h.mustNotContain(body, "javascript:")
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestRequesterSeesOnlyOwn(t *testing.T) {
	h := newHarness(t)
	alice, bob := h.friend("Alice"), h.friend("Bob")
	ha := h.as(alice)
	code, loc, _ := ha.post("/requests", url.Values{"title": {"Alice's thing"}, "requester_id": {itoa(bob.ID)}, "printer_id": {"1"}})
	if code != http.StatusSeeOther {
		t.Fatalf("create: %d", code)
	}
	r, _ := h.store.GetRequest(context.Background(), mustID(t, loc))
	if r.RequesterID != alice.ID || r.PrinterID != nil {
		t.Fatalf("requester could set admin fields: %+v", r)
	}
	hb := h.as(bob)
	if code, _ := hb.get(loc); code != http.StatusForbidden {
		t.Fatalf("bob viewing alice's: %d", code)
	}
	if code, _, _ := hb.post(loc+"/comments", url.Values{"body": {"hi"}}); code != http.StatusForbidden {
		t.Fatalf("bob commenting: %d", code)
	}
	_, body := hb.get("/")
	hb.mustNotContain(body, "Alice&#39;s thing", "data-can-move")
	_, body = ha.get("/")
	ha.mustContain(body, "Alice&#39;s thing")
	ha.mustNotContain(body, "data-can-move")
	if code, _ := ha.get("/printers"); code != http.StatusForbidden {
		t.Fatalf("printers as requester: %d", code)
	}
	if code, _, _ := ha.post(loc+"/move", url.Values{"status": {"printing"}}); code != http.StatusForbidden {
		t.Fatalf("move as requester: %d", code)
	}
	if code, _, _ := ha.post(loc+"/status", url.Values{"status": {"printing"}}); code != http.StatusForbidden {
		t.Fatalf("status as requester: %d", code)
	}
	// Owner may cancel before printing, then may no longer edit.
	if code, _, _ := ha.post(loc+"/status", url.Values{"status": {"cancelled"}}); code != http.StatusSeeOther {
		t.Fatalf("cancel: %d", code)
	}
	if code, _, _ := ha.post(loc, url.Values{"title": {"changed"}}); code != http.StatusForbidden {
		t.Fatalf("edit after cancel: %d", code)
	}
	if code, _, _ := ha.post(loc+"/comments", url.Values{"body": {"still can comment"}}); code != http.StatusSeeOther {
		t.Fatalf("comment after cancel: %d", code)
	}
}

func mustID(t *testing.T, loc string) int64 {
	t.Helper()
	id, err := strconv.ParseInt(strings.TrimPrefix(loc, "/requests/"), 10, 64)
	if err != nil {
		t.Fatalf("location %q", loc)
	}
	return id
}

// TestBoardVersion covers the board's poll: the version it fetches matches
// the page it has, changes when a request is filed, and only follows the
// cards that viewer can see.
func TestBoardVersion(t *testing.T) {
	h := newHarness(t)
	friend := h.as(h.friend("Sam"))
	pageVersion := func(c *harness) string {
		t.Helper()
		_, body := c.get("/")
		m := regexp.MustCompile(`data-version="([^"]+)"`).FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("board has no version: %s", body)
		}
		return m[1]
	}
	version := func(c *harness, q string) string {
		t.Helper()
		code, body := c.get("/board/version" + q)
		if code != http.StatusOK || body == "" {
			t.Fatalf("version: %d %q", code, body)
		}
		return body
	}

	empty := version(h, "")
	if got := pageVersion(h); got != empty {
		t.Fatalf("page version %q, poll says %q", got, empty)
	}
	friendBefore := version(friend, "")

	if code, _, body := h.post("/requests", url.Values{"title": {"Hook"}, "quantity": {"1"}, "timing": {"none"}}); code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", code, body)
	}
	after := version(h, "")
	if after == empty {
		t.Fatal("version did not change after a request was filed")
	}
	if got := pageVersion(h); got != after {
		t.Fatalf("page version %q, poll says %q", got, after)
	}
	// The admin's own request is not on the friend's board.
	if got := version(friend, ""); got != friendBefore {
		t.Fatalf("friend's version changed for a request they cannot see: %q -> %q", friendBefore, got)
	}
	// Filters apply: a material nothing has looks like an empty board.
	if got := version(h, "?material=ASA"); got != empty {
		t.Fatalf("filtered version %q, want the empty board's %q", got, empty)
	}
}
