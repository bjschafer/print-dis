package web

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/printerstatus"
)

// fakeStatus is a StatusSource whose readings tests set.
type fakeStatus struct {
	mu        sync.Mutex
	m         map[int64]printerstatus.Status
	bambuddy  []printerstatus.BambuddyPrinter
	listErr   error
	camInfo   printerstatus.CameraInfo
	camErr    error
	opened    []printerstatus.Target
	streamErr error
}

func (f *fakeStatus) Camera(_ context.Context, t printerstatus.Target) (printerstatus.CameraInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.camInfo, f.camErr
}

func (f *fakeStatus) OpenCamera(_ context.Context, t printerstatus.Target) (*printerstatus.CameraStream, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opened = append(f.opened, t)
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	return &printerstatus.CameraStream{
		ContentType: "multipart/x-mixed-replace; boundary=b",
		Body:        io.NopCloser(strings.NewReader("--b\r\nContent-Type: image/jpeg\r\n\r\nJPEG\r\n")),
	}, nil
}

func (f *fakeStatus) BambuddyPrinters(context.Context, string, string) ([]printerstatus.BambuddyPrinter, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bambuddy, f.listErr
}

func (f *fakeStatus) Status(id int64) (printerstatus.Status, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.m[id]
	return s, ok
}

func (f *fakeStatus) set(id int64, s printerstatus.Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.m == nil {
		f.m = map[int64]printerstatus.Status{}
	}
	f.m[id] = s
}

// statusFixture makes three printers (two with a source, one without), a
// friend, and a printing request of theirs on the first printer.
type statusFixture struct {
	h                *harness
	fake             *fakeStatus
	trident, h2d     int64
	plain            int64
	friend           *core.User
	printingRequest  int64
	requestedRequest int64
}

func newStatusFixture(t *testing.T, withMonitor bool) *statusFixture {
	t.Helper()
	fake := &fakeStatus{}
	h := newHarnessDeps(t, func(d *Deps) {
		if withMonitor {
			d.PrinterStatus = fake
		}
	})
	f := &statusFixture{h: h, fake: fake}
	ctx := context.Background()
	if err := h.store.SaveSettings(ctx, core.Settings{BambuddyURL: "http://bambuddy.test"}); err != nil {
		t.Fatal(err)
	}
	bb := 3
	for _, p := range []*core.Printer{
		{Name: "Trident", BedX: 350, BedY: 350, BedZ: 300, Active: true, MoonrakerURL: "http://trident.test:7125"},
		{Name: "H2D", BedX: 325, BedY: 320, BedZ: 325, Active: true, BambuddyPrinterID: &bb},
		{Name: "Plain", BedX: 200, BedY: 200, BedZ: 200, Active: true},
	} {
		if err := h.store.SavePrinter(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	ps, _ := h.store.ListPrinters(ctx)
	for _, p := range ps {
		switch p.Name {
		case "Trident":
			f.trident = p.ID
		case "H2D":
			f.h2d = p.ID
		case "Plain":
			f.plain = p.ID
		}
	}
	f.friend = h.friend("Alice")
	for _, r := range []*core.Request{
		{Title: "Spool holder", Status: core.StatusPrinting, PrinterID: &f.trident},
		{Title: "Lamp shade", Status: core.StatusRequested, PrinterID: &f.trident},
	} {
		r.RequesterID, r.CreatedByID, r.Quantity, r.Timing = f.friend.ID, f.friend.ID, 1, core.TimingWhenever
		if err := h.store.CreateRequest(ctx, r); err != nil {
			t.Fatal(err)
		}
		if r.Status == core.StatusPrinting {
			f.printingRequest = r.ID
		} else {
			f.requestedRequest = r.ID
		}
	}
	return f
}

func TestBoardShowsPrinterStatus(t *testing.T) {
	f := newStatusFixture(t, true)
	h := f.h
	f.fake.set(f.trident, printerstatus.Status{State: printerstatus.Printing, Job: "Spool_holder_PETG", Progress: 0.42, Remaining: 80 * time.Minute})
	// H2D has a source but has not been read yet.

	code, body := h.get("/")
	if code != http.StatusOK {
		t.Fatalf("board: %d", code)
	}
	strip := between(t, body, `id="printer-strip"`, "</section>")
	h.mustContain(strip, `hx-get="/printers/status"`, "Trident", "Printing", "Spool_holder_PETG", "42<small>%</small>", "1h 20m left", "H2D", "Checking…",
		`href="/printers/`+strconv.FormatInt(f.trident, 10)+`/camera" data-camera`)
	if strings.Contains(strip, "Plain") {
		t.Error("a printer with no source is in the strip")
	}
	// The printing card on Trident shows it; the requested one does not.
	h.mustContain(body, `class="card-live live-p`+strconv.FormatInt(f.trident, 10)+`"`)
	if n := strings.Count(body, `class="card-live `); n != 1 {
		t.Errorf("%d live lines on cards, want 1", n)
	}

	// The poll returns the strip and an out-of-band update for the cards.
	f.fake.set(f.trident, printerstatus.Status{State: printerstatus.Finished, Job: "Spool_holder_PETG", Progress: 1})
	code, body = h.get("/printers/status")
	if code != http.StatusOK {
		t.Fatalf("status fragment: %d", code)
	}
	h.mustContain(body, `id="printer-strip"`, "Finished",
		`hx-swap-oob="outerHTML:.s-printing .live-p`+strconv.FormatInt(f.trident, 10)+`"`)
	h.mustNotContain(body, "<html", "42<small>",
		// H2D has no printing card, so no update for one.
		`live-p`+strconv.FormatInt(f.h2d, 10))
}

func TestPrinterStatusIsAdminOnly(t *testing.T) {
	f := newStatusFixture(t, true)
	f.fake.set(f.trident, printerstatus.Status{State: printerstatus.Printing, Job: "Someone_elses_job", Progress: 0.5})
	alice := f.h.as(f.friend)
	_, body := alice.get("/")
	alice.mustContain(body, "Spool holder")
	alice.mustNotContain(body, "printer-strip", "card-live", "Someone_elses_job")
	_, body = alice.get("/requests/" + strconv.FormatInt(f.printingRequest, 10))
	alice.mustNotContain(body, "Someone_elses_job", "live-state")
	if code, _ := alice.get("/printers/status"); code != http.StatusForbidden {
		t.Errorf("requester status fragment: %d", code)
	}
}

func TestNoMonitorNoStrip(t *testing.T) {
	f := newStatusFixture(t, false)
	_, body := f.h.get("/")
	f.h.mustNotContain(body, "printer-strip", "card-live")
	code, body := f.h.get("/printers/status")
	if code != http.StatusOK || !strings.Contains(body, "hidden") || strings.Contains(body, "hx-get") {
		t.Errorf("fragment without a monitor should be an empty strip that stops polling: %d %s", code, body)
	}
}

func TestDetailAndPrintersPageShowStatus(t *testing.T) {
	f := newStatusFixture(t, true)
	h := f.h
	f.fake.set(f.trident, printerstatus.Status{State: printerstatus.Paused, Job: "Spool_holder_PETG", Progress: 0.7, Remaining: 5 * time.Minute})
	f.fake.set(f.h2d, printerstatus.Status{State: printerstatus.Offline})
	_, body := h.get("/requests/" + strconv.FormatInt(f.requestedRequest, 10))
	h.mustContain(body, "Paused", "70%", "5m left", "Offline")
	_, body = h.get("/printers")
	h.mustContain(body, "Moonraker", "Bambuddy: #3", "Paused", "Spool_holder_PETG", `value="http://trident.test:7125"`)
}

func TestPrinterFormStatusSource(t *testing.T) {
	f := newStatusFixture(t, true)
	h := f.h
	base := url.Values{"name": {"Pandora"}, "bed_x": {"120"}, "bed_y": {"120"}, "bed_z": {"120"}, "active": {"on"}}
	with := func(kv ...string) url.Values {
		v := url.Values{}
		for k, vs := range base {
			v[k] = vs
		}
		for i := 0; i+1 < len(kv); i += 2 {
			v.Set(kv[i], kv[i+1])
		}
		return v
	}
	for _, tt := range []struct {
		name string
		form url.Values
		want string
	}{
		{"both", with("moonraker_url", "http://pb.test:7125", "bambuddy_printer_id", "2"), "not both"},
		{"bad url", with("moonraker_url", "pb.test:7125"), "Moonraker address"},
		{"bad id", with("bambuddy_printer_id", "two"), "Choose which Bambuddy printer"},
		{"bambuddy chosen without one", with("status_source", "bambuddy"), "Choose which Bambuddy printer"},
		{"moonraker chosen without address", with("status_source", "moonraker"), "Moonraker address"},
		{"unknown source", with("status_source", "octoprint"), "Choose a live status source"},
	} {
		code, _, body := h.post("/printers", tt.form)
		if code != http.StatusUnprocessableEntity || !strings.Contains(body, tt.want) {
			t.Errorf("%s: %d, want 422 with %q", tt.name, code, tt.want)
		}
	}
	if code, _, body := h.post("/printers", with("moonraker_url", "http://pb.test:7125/")); code != http.StatusSeeOther {
		t.Fatalf("save: %d %s", code, body)
	}
	// Switching a printer to Bambuddy, then clearing it.
	id := strconv.FormatInt(f.trident, 10)
	trident := url.Values{"name": {"Trident"}, "bed_x": {"350"}, "bed_y": {"350"}, "bed_z": {"300"}, "active": {"on"}, "bambuddy_printer_id": {"5"}}
	if code, _, body := h.post("/printers/"+id, trident); code != http.StatusSeeOther {
		t.Fatalf("switch: %d %s", code, body)
	}
	trident.Del("bambuddy_printer_id")
	if code, _, body := h.post("/printers/"+id, trident); code != http.StatusSeeOther {
		t.Fatalf("clear: %d %s", code, body)
	}
	ps, _ := h.store.ListPrinters(context.Background())
	for _, p := range ps {
		switch p.Name {
		case "Pandora":
			if p.MoonrakerURL != "http://pb.test:7125" || p.BambuddyPrinterID != nil {
				t.Errorf("Pandora: %q %v", p.MoonrakerURL, p.BambuddyPrinterID)
			}
		case "Trident":
			if p.HasStatusSource() {
				t.Errorf("Trident still has a source: %q %v", p.MoonrakerURL, p.BambuddyPrinterID)
			}
		}
	}
}

func between(t *testing.T, s, from, to string) string {
	t.Helper()
	i := strings.Index(s, from)
	if i < 0 {
		t.Fatalf("missing %q", from)
	}
	j := strings.Index(s[i:], to)
	if j < 0 {
		t.Fatalf("missing %q after %q", to, from)
	}
	return s[i : i+j]
}

func TestBambuddySettings(t *testing.T) {
	f := newStatusFixture(t, true)
	h := f.h
	ctx := context.Background()
	settings := func() core.Settings {
		t.Helper()
		st, err := h.store.GetSettings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}

	// The API address is trimmed to the origin; the key is saved.
	code, loc, body := h.post("/printers/bambuddy", url.Values{"bambuddy_url": {"http://bambuddy.lan:8000/api/v1/"}, "bambuddy_api_key": {"bb_secret"}})
	if code != http.StatusSeeOther || loc != "/printers" {
		t.Fatalf("save: %d %s %s", code, loc, body)
	}
	if st := settings(); st.BambuddyURL != "http://bambuddy.lan:8000" || st.BambuddyAPIKey != "bb_secret" {
		t.Fatalf("saved %+v", st)
	}
	_, body = h.get("/printers")
	h.mustContain(body, `value="http://bambuddy.lan:8000"`, "Saved. Leave blank to keep it.", "Remove the saved key")
	h.mustNotContain(body, "bb_secret", "Bambuddy connection</a> first")

	// A blank key keeps the saved one; a bad address is refused and keeps both.
	h.post("/printers/bambuddy", url.Values{"bambuddy_url": {"http://bambuddy.lan:8001"}})
	if st := settings(); st.BambuddyURL != "http://bambuddy.lan:8001" || st.BambuddyAPIKey != "bb_secret" {
		t.Fatalf("blank key: %+v", st)
	}
	code, _, body = h.post("/printers/bambuddy", url.Values{"bambuddy_url": {"bambuddy.lan:8000"}, "bambuddy_api_key": {"bb_other"}})
	if code != http.StatusUnprocessableEntity || !strings.Contains(body, "Bambuddy address") || !strings.Contains(body, `value="bambuddy.lan:8000"`) {
		t.Errorf("bad address: %d", code)
	}
	h.mustNotContain(body, "bb_secret", "bb_other")
	if st := settings(); st.BambuddyURL != "http://bambuddy.lan:8001" || st.BambuddyAPIKey != "bb_secret" {
		t.Errorf("failed save changed settings: %+v", st)
	}

	// Clearing the key, then the address.
	h.post("/printers/bambuddy", url.Values{"bambuddy_url": {""}, "clear_api_key": {"on"}})
	if st := settings(); st != (core.Settings{}) {
		t.Errorf("cleared: %+v", st)
	}
	_, body = h.get("/printers")
	h.mustContain(body, "Bambuddy connection</a> first")

	// Requesters cannot change it.
	if code, _, _ := h.as(f.friend).post("/printers/bambuddy", url.Values{"bambuddy_url": {"http://evil.test"}}); code != http.StatusForbidden {
		t.Errorf("requester: %d", code)
	}
}

func TestPrinterFormBambuddyPicker(t *testing.T) {
	f := newStatusFixture(t, true)
	h := f.h
	f.fake.bambuddy = []printerstatus.BambuddyPrinter{{ID: 3, Name: "H2D", Model: "H2D"}, {ID: 7, Name: "Spare P1S", Model: "P1S"}}

	_, body := h.get("/printers")
	h.mustContain(body, `name="status_source" value="bambuddy" checked`, `name="status_source" value="moonraker" checked`,
		`<option value="7" >Spare P1S (P1S)</option>`, `<option value="3" selected>H2D (H2D)</option>`, "Bambuddy: H2D")

	// Choosing Bambuddy keeps only the picked printer, whatever else was typed.
	id := strconv.FormatInt(f.trident, 10)
	form := url.Values{"name": {"Trident"}, "bed_x": {"350"}, "bed_y": {"350"}, "bed_z": {"300"}, "active": {"on"},
		"status_source": {"bambuddy"}, "bambuddy_printer_id": {"7"}, "moonraker_url": {"http://trident.test:7125"}}
	if code, _, body := h.post("/printers/"+id, form); code != http.StatusSeeOther {
		t.Fatalf("pick: %d %s", code, body)
	}
	p, _ := h.store.GetPrinter(context.Background(), f.trident)
	if p.MoonrakerURL != "" || p.BambuddyPrinterID == nil || *p.BambuddyPrinterID != 7 {
		t.Errorf("after picking: %q %v", p.MoonrakerURL, p.BambuddyPrinterID)
	}
	// None clears it.
	form.Set("status_source", "")
	h.post("/printers/"+id, form)
	if p, _ := h.store.GetPrinter(context.Background(), f.trident); p.HasStatusSource() {
		t.Errorf("none kept a source: %q %v", p.MoonrakerURL, p.BambuddyPrinterID)
	}
	// A failed save shows the chosen source again.
	form.Set("status_source", "moonraker")
	form.Set("moonraker_url", "")
	code, _, body := h.post("/printers/"+id, form)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("missing address: %d", code)
	}
	h.mustContain(body, `value="moonraker" checked`)

	// Bambuddy down: say so and offer the id by hand.
	f.fake.bambuddy, f.fake.listErr = nil, errors.New("down")
	_, body = h.get("/printers")
	h.mustContain(body, "Could not reach Bambuddy", `type="number" min="1" value="3"`)

	// No connection yet: point at it.
	if err := h.store.SaveSettings(context.Background(), core.Settings{}); err != nil {
		t.Fatal(err)
	}
	_, body = h.get("/printers")
	h.mustContain(body, "Bambuddy connection</a> first")
}

func TestCamera(t *testing.T) {
	f := newStatusFixture(t, true)
	h := f.h
	f.fake.camInfo = printerstatus.CameraInfo{Name: "Bed cam", Rotation: 180, FlipH: true}
	trident := "/printers/" + strconv.FormatInt(f.trident, 10)

	// The page, and the bare view for the dialog.
	code, body := h.get(trident + "/camera")
	if code != http.StatusOK {
		t.Fatalf("camera page: %d", code)
	}
	h.mustContain(body, "<html", "Trident", "Bed cam", `src="`+trident+`/camera/stream"`, "transform: rotate(180deg) scaleX(-1)")
	req, _ := http.NewRequest(http.MethodGet, h.srv.URL+trident+"/camera", nil)
	req.Header.Set("HX-Request", "true")
	res, err := h.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || strings.Contains(string(b), "<html") || !strings.Contains(string(b), "camera-img") {
		t.Errorf("dialog view: %d %s", res.StatusCode, b)
	}

	// The stream is relayed with its content type.
	res, err = h.c.Get(h.srv.URL + trident + "/camera/stream")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "multipart/x-mixed-replace; boundary=b" || !strings.Contains(string(b), "JPEG") {
		t.Errorf("stream: %d %q %q", res.StatusCode, res.Header.Get("Content-Type"), b)
	}
	if len(f.fake.opened) != 1 || f.fake.opened[0].MoonrakerURL != "http://trident.test:7125" {
		t.Errorf("opened %+v", f.fake.opened)
	}
	// The H2D's camera goes through Bambuddy with the saved connection.
	res, _ = h.c.Get(h.srv.URL + "/printers/" + strconv.FormatInt(f.h2d, 10) + "/camera/stream")
	_ = res.Body.Close()
	if got := f.fake.opened[1]; got.BambuddyID != 3 || got.BambuddyURL != "http://bambuddy.test" {
		t.Errorf("bambuddy target %+v", got)
	}

	// Failures are a sentence, not a crash.
	f.fake.camErr, f.fake.streamErr = errors.New("no webcams"), errors.New("refused")
	_, body = h.get(trident + "/camera")
	h.mustContain(body, "The camera is not available right now.")
	h.mustNotContain(body, "camera-img")
	res, _ = h.c.Get(h.srv.URL + trident + "/camera/stream")
	b, _ = io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadGateway || !strings.Contains(string(b), "not available") {
		t.Errorf("failed stream: %d %s", res.StatusCode, b)
	}

	// A printer without a source has no camera; requesters see none.
	if code, _ := h.get("/printers/" + strconv.FormatInt(f.plain, 10) + "/camera"); code != http.StatusNotFound {
		t.Errorf("no source: %d", code)
	}
	if code, _ := h.get("/printers/999999/camera/stream"); code != http.StatusNotFound {
		t.Errorf("missing printer: %d", code)
	}
	alice := h.as(f.friend)
	for _, path := range []string{trident + "/camera", trident + "/camera/stream"} {
		if code, _ := alice.get(path); code != http.StatusForbidden {
			t.Errorf("requester %s: %d", path, code)
		}
	}
}
