package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/bjschafer/print-dis/internal/spoolman"
	"github.com/bjschafer/print-dis/internal/store"
)

// fakeFilaments is a FilamentSource whose answer tests can change.
type fakeFilaments struct {
	mu   sync.Mutex
	opts []spoolman.FilamentOption
	err  error
}

func (f *fakeFilaments) Filaments(context.Context) ([]spoolman.FilamentOption, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opts, f.err
}

func (f *fakeFilaments) set(opts []spoolman.FilamentOption, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opts, f.err = opts, err
}

func sampleFilaments() []spoolman.FilamentOption {
	return []spoolman.FilamentOption{
		{ID: 1, Vendor: "Elegoo", Name: "Blue", Material: "PETG", ColorHex: "#0000ff", Label: "Elegoo Blue", InStock: false},
		{ID: 2, Vendor: "Prusament", Name: "Galaxy Black", Material: "PLA", ColorHex: "#1a1a1a", Label: "Prusament Galaxy Black", InStock: true},
		{ID: 3, Vendor: "Polymaker", Name: "ASA Grey", Material: "ASA", ColorHex: "#888888", Label: "Polymaker ASA Grey", InStock: true},
	}
}

func filamentHarness(t *testing.T, src *fakeFilaments) *harness {
	t.Helper()
	return newHarnessDeps(t, func(d *Deps) { d.Filaments = src })
}

func TestFilamentPickerListsGroups(t *testing.T) {
	h := filamentHarness(t, &fakeFilaments{opts: sampleFilaments()})
	code, body := h.get("/requests/new")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	h.mustContain(body, `name="spoolman_filament_id"`, "Any filament", `name="color_note"`,
		`<optgroup label="ASA">`, `<optgroup label="PETG">`, `<optgroup label="PLA">`,
		"Elegoo Blue (out of stock)", `data-material="PLA"`, `data-hex="#1a1a1a"`)
	h.mustNotContain(body, "Prusament Galaxy Black (out of stock)", "unavailable", `name="material"`)
	if strings.Index(body, `label="ASA"`) > strings.Index(body, `label="PETG"`) ||
		strings.Index(body, `label="PETG"`) > strings.Index(body, `label="PLA"`) {
		t.Error("material groups are not sorted")
	}
}

func TestFilamentPickSnapshots(t *testing.T) {
	src := &fakeFilaments{opts: sampleFilaments()}
	h := filamentHarness(t, src)
	code, loc, body := h.post("/requests", url.Values{
		"title": {"Bracket"}, "spoolman_filament_id": {"2"}, "color_note": {"glossy"},
		// A crafted form must not set the snapshot.
		"filament_label": {"Evil <b>label</b>"}, "filament_color_hex": {"#ff0000"}, "material": {"abs"},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", code, body)
	}
	reqs, _ := h.store.ListRequests(context.Background(), store.Filter{})
	if len(reqs) != 1 {
		t.Fatalf("got %d requests", len(reqs))
	}
	r := reqs[0]
	if r.SpoolmanFilamentID == nil || *r.SpoolmanFilamentID != 2 || r.FilamentLabel != "Prusament Galaxy Black" ||
		r.FilamentColorHex == nil || *r.FilamentColorHex != "#1a1a1a" || r.Material != "PLA" || r.ColorNote != "glossy" {
		t.Fatalf("snapshot wrong: %+v", r)
	}

	// The edit form preselects the pick.
	_, body = h.get(loc)
	h.mustContain(body, `value="2" data-material="PLA" data-hex="#1a1a1a" selected`)

	// Picking another filament replaces the snapshot.
	form := url.Values{"title": {"Bracket"}, "spoolman_filament_id": {"3"}, "requester_id": {itoa(r.RequesterID)}}
	if code, _, body := h.post(loc, form); code != http.StatusSeeOther {
		t.Fatalf("edit: %d %s", code, body)
	}
	got, _ := h.store.GetRequest(context.Background(), r.ID)
	if got.FilamentLabel != "Polymaker ASA Grey" || got.Material != "ASA" {
		t.Fatalf("after re-pick: %+v", got)
	}

	// "Any" clears it all.
	form.Set("spoolman_filament_id", "")
	if code, _, body := h.post(loc, form); code != http.StatusSeeOther {
		t.Fatalf("edit any: %d %s", code, body)
	}
	got, _ = h.store.GetRequest(context.Background(), r.ID)
	if got.SpoolmanFilamentID != nil || got.FilamentLabel != "" || got.FilamentColorHex != nil || got.Material != "" {
		t.Fatalf("after any: %+v", got)
	}
}

func TestFilamentUnknownPick(t *testing.T) {
	h := filamentHarness(t, &fakeFilaments{opts: sampleFilaments()})
	code, _, body := h.post("/requests", url.Values{"title": {"X"}, "spoolman_filament_id": {"99"}})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown filament: %d", code)
	}
	h.mustContain(body, "not in the list any more")
	code, _, _ = h.post("/requests", url.Values{"title": {"X"}, "spoolman_filament_id": {"abc"}})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("bad filament id: %d", code)
	}
}

func TestFilamentSourceDown(t *testing.T) {
	src := &fakeFilaments{err: errors.New("spoolman is down")}
	h := filamentHarness(t, src)
	code, body := h.get("/requests/new")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	h.mustContain(body, "filament list is unavailable", `name="material"`, `name="color_note"`)
	h.mustNotContain(body, `name="spoolman_filament_id"`)

	// Free text still saves.
	code, _, body = h.post("/requests", url.Values{"title": {"Free"}, "material": {"petg"}, "color_note": {"red"}})
	if code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", code, body)
	}

	// Without a source at all there is no note.
	plain := newHarness(t)
	_, body = plain.get("/requests/new")
	plain.mustContain(body, `name="material"`)
	plain.mustNotContain(body, "unavailable", "spoolman_filament_id")
}

func TestFilamentVanishedStillDisplays(t *testing.T) {
	src := &fakeFilaments{opts: sampleFilaments()}
	h := filamentHarness(t, src)
	code, loc, body := h.post("/requests", url.Values{"title": {"Bracket"}, "spoolman_filament_id": {"1"}})
	if code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", code, body)
	}
	reqs, _ := h.store.ListRequests(context.Background(), store.Filter{})
	rid := reqs[0].RequesterID

	// The filament is deleted from Spoolman, then Spoolman goes away.
	src.set(sampleFilaments()[1:], nil)
	check := func() {
		t.Helper()
		_, body := h.get(loc)
		h.mustContain(body, "Elegoo Blue", `background: #0000ff`)
		_, body = h.get("/")
		h.mustContain(body, "Elegoo Blue", `background: #0000ff`)
		_, body = h.get("/requests")
		h.mustContain(body, "Elegoo Blue", `background: #0000ff`)
	}
	check()
	_, body = h.get(loc)
	h.mustContain(body, "(no longer in Spoolman)")

	// Saving the edit form keeps the snapshot.
	form := url.Values{"title": {"Bracket 2"}, "spoolman_filament_id": {"1"}, "requester_id": {itoa(rid)}}
	if code, _, body := h.post(loc, form); code != http.StatusSeeOther {
		t.Fatalf("edit: %d %s", code, body)
	}
	got, _ := h.store.GetRequest(context.Background(), reqs[0].ID)
	if got.Title != "Bracket 2" || got.FilamentLabel != "Elegoo Blue" || got.Material != "PETG" {
		t.Fatalf("snapshot lost: %+v", got)
	}

	src.set(nil, errors.New("down"))
	check()
	_, body = h.get(loc)
	h.mustContain(body, "filament list is unavailable")
}
