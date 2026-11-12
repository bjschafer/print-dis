package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/bjschafer/print-dis/internal/core"
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
		{ID: 1, Vendor: "Elegoo", Name: "Blue", Material: "PETG", ColorHex: "#0000ff", Hexes: []string{"#0000ff"}, Label: "Elegoo Blue", InStock: false},
		{ID: 2, Vendor: "Prusament", Name: "Galaxy Black", Material: "PLA", ColorHex: "#1a1a1a", Hexes: []string{"#1a1a1a"}, Label: "Prusament Galaxy Black", InStock: true, Grams: 640},
		{ID: 3, Vendor: "Polymaker", Name: "ASA Grey", Material: "ASA", ColorHex: "#888888", Hexes: []string{"#888888"}, Label: "Polymaker ASA Grey", InStock: true, Grams: 1200},
		{ID: 4, Vendor: "Polymaker", Name: "Sky Blue", Material: "PLA", ColorHex: "#3070e0", Hexes: []string{"#3070e0"}, Label: "Polymaker Sky Blue", InStock: true, Grams: 800},
		{ID: 5, Vendor: "Eryone", Name: "Rainbow", Material: "PLA", ColorHex: "#ff0000", Hexes: []string{"#ff0000", "#00ff00"}, Label: "Eryone Rainbow", InStock: true, Grams: 50},
	}
}

func filamentHarness(t *testing.T, src *fakeFilaments) *harness {
	t.Helper()
	return newHarnessDeps(t, func(d *Deps) { d.Filaments = src })
}

// colorForm is a request form with the color picker's marker set.
func colorForm(kv ...string) url.Values {
	v := url.Values{"title": {"Bracket"}, "colors_form": {"1"}}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	return v
}

func onlyRequest(t *testing.T, h *harness) core.Request {
	t.Helper()
	reqs, err := h.store.ListRequests(context.Background(), store.Filter{})
	if err != nil || len(reqs) != 1 {
		t.Fatalf("got %d requests, err %v", len(reqs), err)
	}
	return reqs[0]
}

func TestColorPickerLists(t *testing.T) {
	h := filamentHarness(t, &fakeFilaments{opts: sampleFilaments()})
	code, body := h.get("/requests/new")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	h.mustContain(body,
		// material chips: the common ones and Spoolman's
		`name="material" value="" checked`, `name="material" value="PETG"`, `name="material" value="ASA"`,
		`name="material" value="TPU"`, `name="material_other"`,
		// rough colors, exact, and the shelf
		`name="color_0" value="" checked`, `value="c:blue"`, `background: #1e66d0`, `name="color_hex_0"`,
		`value="f:2" data-fil-material="PLA"`, "Prusament Galaxy Black", "640 g left", "1.2 kg left", "out of stock",
		"(4 in stock)", `name="color_note_0"`, `data-slot-template`, `name="color___N__"`,
		// a multi-color filament is a pie
		"conic-gradient(from 0deg, #ff0000 0.00% 50.00%, #00ff00 50.00% 100.00%)",
		"Several colors or materials in one print")
	h.mustNotContain(body, `name="spoolman_filament_id"`, "unavailable", "ZgotmplZ")
	if strings.Index(body, "<h4>ASA</h4>") > strings.Index(body, "<h4>PETG</h4>") ||
		strings.Index(body, "<h4>PETG</h4>") > strings.Index(body, "<h4>PLA</h4>") {
		t.Error("material groups are not sorted")
	}
}

func TestColorPickSnapshots(t *testing.T) {
	h := filamentHarness(t, &fakeFilaments{opts: sampleFilaments()})
	code, loc, body := h.post("/requests", colorForm("color_0", "f:2", "color_note_0", "glossy",
		// A crafted form must not set the snapshot.
		"filament_label", "Evil <b>label</b>", "filament_color_hex", "#ff0000", "color_hex_0", "#ff0000"))
	if code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", code, body)
	}
	r := onlyRequest(t, h)
	if len(r.Colors) != 1 {
		t.Fatalf("colors = %+v", r.Colors)
	}
	c := r.Colors[0]
	if c.FilamentID == nil || *c.FilamentID != 2 || c.Label != "Prusament Galaxy Black" || c.Hex() != "#1a1a1a" ||
		c.Material != "PLA" || c.Note != "glossy" || r.Material != "PLA" {
		t.Fatalf("snapshot wrong: %+v %+v", r, c)
	}

	// The edit form shows the pick, with the shelf open.
	_, body = h.get(loc)
	h.mustContain(body, `value="f:2" data-fil-material="PLA" checked`, `<details class="shelf" open>`, `value="glossy"`,
		`name="material" value="PLA" checked`)

	// Picking another filament replaces the snapshot; the material was
	// derived, so it is cleared to follow the new pick.
	form := colorForm("color_0", "f:3", "material", "", "requester_id", itoa(r.RequesterID))
	if code, _, body := h.post(loc, form); code != http.StatusSeeOther {
		t.Fatalf("edit: %d %s", code, body)
	}
	got, _ := h.store.GetRequest(context.Background(), r.ID)
	if len(got.Colors) != 1 || got.Colors[0].Label != "Polymaker ASA Grey" || got.Material != "ASA" {
		t.Fatalf("after re-pick: %+v", got)
	}
	_, body = h.get(loc)
	h.mustContain(body, "changed material, color")

	// Any clears it all.
	form.Set("color_0", "")
	if code, _, body := h.post(loc, form); code != http.StatusSeeOther {
		t.Fatalf("edit any: %d %s", code, body)
	}
	got, _ = h.store.GetRequest(context.Background(), r.ID)
	if len(got.Colors) != 0 || got.Material != "" {
		t.Fatalf("after any: %+v", got)
	}

	// A form without the picker leaves colors alone.
	form = colorForm("color_0", "c:red", "requester_id", itoa(r.RequesterID))
	if code, _, body := h.post(loc, form); code != http.StatusSeeOther {
		t.Fatalf("edit red: %d %s", code, body)
	}
	if code, _, body := h.post(loc, url.Values{"title": {"Bracket"}, "requester_id": {itoa(r.RequesterID)}}); code != http.StatusSeeOther {
		t.Fatalf("edit without picker: %d %s", code, body)
	}
	got, _ = h.store.GetRequest(context.Background(), r.ID)
	if len(got.Colors) != 1 || got.Colors[0].Rough != "red" {
		t.Fatalf("picker-less edit changed colors: %+v", got.Colors)
	}
}

func TestSeveralColors(t *testing.T) {
	h := filamentHarness(t, &fakeFilaments{opts: sampleFilaments()})
	// Slot keys need not be contiguous; a blank slot is dropped.
	code, loc, body := h.post("/requests", colorForm(
		"color_0", "c:red", "color_hex_0", "#808080",
		"color_2", "", "color_hex_2", "#808080",
		"color_3", "f:4", "color_note_3", "for the base",
		"color_7", "hex", "color_hex_7", "#12AB34",
		"color_9", "", "color_note_9", "whatever's left over",
	))
	if code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", code, body)
	}
	r := onlyRequest(t, h)
	if len(r.Colors) != 4 {
		t.Fatalf("colors = %+v", r.Colors)
	}
	want := []struct{ name, hex, note string }{
		{"Red", "#d32f2f", ""},
		{"Polymaker Sky Blue", "#3070e0", "for the base"},
		{"Exact color", "#12ab34", ""},
		{"Any color", "", "whatever's left over"},
	}
	for i, w := range want {
		if c := r.Colors[i]; c.Name() != w.name || c.Hex() != w.hex || c.Note != w.note {
			t.Errorf("color %d = %+v, want %+v", i, c, w)
		}
	}
	if r.Material != "PLA" {
		t.Errorf("material = %q", r.Material)
	}

	// The board stripes the card with every color, top to bottom.
	_, body = h.get("/")
	h.mustContain(body, "--fil: linear-gradient(to bottom, #d32f2f 0.00% 33.33%, #3070e0 33.33% 66.67%, #12ab34 66.67% 100.00%)",
		"Polymaker Sky Blue", "for the base", "whatever&#39;s left over")

	// The edit form numbers the slots and keeps each choice.
	_, body = h.get(loc)
	h.mustContain(body, "Color 1", "Color 4", `name="color_0" value="c:red" checked`, `name="color_1" value="f:4" data-fil-material="PLA" checked`,
		`name="color_2" value="hex" checked`, `name="color_hex_2" value="#12ab34"`, `name="color_3" value="" checked`)

	// Too many colors.
	form := colorForm()
	for i := range core.MaxColors + 1 {
		form.Set("color_"+itoa(int64(i)), "c:blue")
	}
	if code, _, body := h.post("/requests", form); code != http.StatusUnprocessableEntity || !strings.Contains(body, "at most 8 colors") {
		t.Fatalf("too many: %d", code)
	}
}

func TestMaterialOnly(t *testing.T) {
	h := filamentHarness(t, &fakeFilaments{opts: sampleFilaments()})
	for _, tc := range []struct {
		form url.Values
		want string
	}{
		{colorForm("material", "PETG"), "PETG"},
		{colorForm("material", "other", "material_other", "pa12-cf"), "PA12-CF"},
		{colorForm("material", "", "material_other", "tpu 95a"), "TPU 95A"},
		{colorForm("material", "PETG", "material_other", "ignored"), "PETG"},
		// A named material wins over the picked filament's.
		{colorForm("material", "PETG", "color_0", "f:2"), "PETG"},
	} {
		code, loc, body := h.post("/requests", tc.form)
		if code != http.StatusSeeOther {
			t.Fatalf("create %v: %d %s", tc.form, code, body)
		}
		id, _ := optID(strings.TrimPrefix(loc, "/requests/"))
		got, _ := h.store.GetRequest(context.Background(), id)
		if got.Material != tc.want {
			t.Errorf("%v: material %q, want %q", tc.form, got.Material, tc.want)
		}
	}
	// A material that is not a chip shows as "other" on the edit form.
	reqs, _ := h.store.ListRequests(context.Background(), store.Filter{Material: "PA12-CF"})
	_, body := h.get("/requests/" + itoa(reqs[0].ID))
	h.mustContain(body, `value="other" checked`, `name="material_other" value="PA12-CF"`)
}

func TestColorBadPicks(t *testing.T) {
	h := filamentHarness(t, &fakeFilaments{opts: sampleFilaments()})
	for _, tc := range []struct{ value, hex, want string }{
		{"f:99", "", "not in the list any more"},
		{"f:abc", "", "Pick a filament from the list"},
		{"c:mauve", "", "Pick a color from the list"},
		{"hex", "#zzzzzz", "Pick the exact color again"},
		{"spoolman", "", "Pick a color from the list"},
	} {
		code, _, body := h.post("/requests", colorForm("color_0", tc.value, "color_hex_0", tc.hex))
		if code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: %d", tc.value, code)
		}
		h.mustContain(body, tc.want)
	}
}

func TestAddColorWithoutScript(t *testing.T) {
	h := filamentHarness(t, &fakeFilaments{opts: sampleFilaments()})
	code, _, body := h.post("/requests", colorForm("color_0", "c:green", "add_color", "1"))
	if code != http.StatusOK {
		t.Fatalf("add color: %d", code)
	}
	h.mustContain(body, `name="color_0" value="c:green" checked`, `name="color_1" value="" checked`, "Color 2")
	if reqs, _ := h.store.ListRequests(context.Background(), store.Filter{}); len(reqs) != 0 {
		t.Fatal("add color saved the request")
	}

	// On the edit form too.
	code, loc, _ := h.post("/requests", colorForm("color_0", "c:green"))
	if code != http.StatusSeeOther {
		t.Fatalf("create: %d", code)
	}
	r := onlyRequest(t, h)
	code, _, body = h.post(loc, colorForm("color_0", "c:green", "add_color", "1", "requester_id", itoa(r.RequesterID)))
	if code != http.StatusOK {
		t.Fatalf("add color on edit: %d", code)
	}
	h.mustContain(body, `name="color_1" value="" checked`)
}

func TestColorSourceDown(t *testing.T) {
	src := &fakeFilaments{err: errors.New("spoolman is down")}
	h := filamentHarness(t, src)
	code, body := h.get("/requests/new")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	h.mustContain(body, "filament list is unavailable", `name="material" value="PLA"`, `value="c:blue"`, `name="color_note_0"`)
	h.mustNotContain(body, `value="f:`, "in stock)")

	// Rough colors still save.
	code, _, body = h.post("/requests", colorForm("material", "PETG", "color_0", "c:red"))
	if code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", code, body)
	}

	// Without a source at all there is no note, but the rest is the same.
	plain := newHarness(t)
	_, body = plain.get("/requests/new")
	plain.mustContain(body, `name="material" value="TPU"`, `value="c:red"`)
	plain.mustNotContain(body, "unavailable", `value="f:`)
}

func TestFilamentVanishedStillDisplays(t *testing.T) {
	src := &fakeFilaments{opts: sampleFilaments()}
	h := filamentHarness(t, src)
	code, loc, body := h.post("/requests", colorForm("color_0", "f:1"))
	if code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", code, body)
	}
	r := onlyRequest(t, h)

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
	h.mustContain(body, "no longer in Spoolman", `name="color_0" value="f:1" data-fil-material="PETG" checked`)

	// Saving the edit form keeps the snapshot.
	form := colorForm("title", "Bracket 2", "color_0", "f:1", "requester_id", itoa(r.RequesterID))
	if code, _, body := h.post(loc, form); code != http.StatusSeeOther {
		t.Fatalf("edit: %d %s", code, body)
	}
	got, _ := h.store.GetRequest(context.Background(), r.ID)
	if got.Title != "Bracket 2" || len(got.Colors) != 1 || got.Colors[0].Label != "Elegoo Blue" || got.Material != "PETG" {
		t.Fatalf("snapshot lost: %+v", got)
	}

	src.set(nil, errors.New("down"))
	check()
	_, body = h.get(loc)
	h.mustContain(body, "filament list is unavailable", "kept as it was")
}

func TestShelfMatches(t *testing.T) {
	h := filamentHarness(t, &fakeFilaments{opts: sampleFilaments()})
	code, loc, body := h.post("/requests", colorForm("color_0", "c:blue", "color_1", "f:2"))
	if code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", code, body)
	}
	_, body = h.get(loc)
	h.mustContain(body, "On the shelf", "Polymaker Sky Blue", "800 g left")
	// Out of stock filaments, and picked ones, are not suggested.
	shelf := body[strings.Index(body, "On the shelf"):]
	shelf = shelf[:strings.Index(shelf, "</section>")]
	if strings.Contains(shelf, "Elegoo Blue") || strings.Contains(shelf, "Galaxy Black") {
		t.Errorf("shelf suggests the wrong things: %s", shelf)
	}

	// Requesters do not see it.
	friend := h.friend("Sam")
	fh := h.as(friend)
	code, loc, body = fh.post("/requests", colorForm("color_0", "c:blue"))
	if code != http.StatusSeeOther {
		t.Fatalf("friend create: %d %s", code, body)
	}
	_, body = fh.get(loc)
	fh.mustContain(body, "Blue")
	fh.mustNotContain(body, "On the shelf")
}

func TestBoardCollapsedColumns(t *testing.T) {
	h := newHarness(t)
	_, body := h.get("/")
	h.mustContain(body, `class="column s-needs_info" data-column="needs_info"`, `data-col-toggle aria-expanded="true"`)
	u, _ := url.Parse(h.srv.URL)
	h.c.Jar.SetCookies(u, []*http.Cookie{{Name: collapsedCookie, Value: "needs_info.bogus.ready"}})
	_, body = h.get("/")
	h.mustContain(body, `class="column s-needs_info collapsed"`, `class="column s-ready collapsed"`, `aria-label="Show Needs info"`)
	h.mustNotContain(body, `class="column s-requested collapsed"`)
}
