package web

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/printerstatus"
)

// TestScreenshots regenerates the README images in docs/images from made-up
// data: fake friends, printers, filaments and printer status, served by the
// real handlers and captured with headless Chromium. It skips unless
// SCREENSHOTS=1 so `make check` never rewrites the PNGs. Run it with
// `make screenshots` (needs `make db` and chromium on PATH).
func TestScreenshots(t *testing.T) {
	if os.Getenv("SCREENSHOTS") != "1" {
		t.Skip("SCREENSHOTS=1 not set")
	}
	chrome := os.Getenv("CHROMIUM")
	if chrome == "" {
		for _, name := range []string{"chromium", "chromium-browser", "google-chrome-stable", "google-chrome"} {
			if p, err := exec.LookPath(name); err == nil {
				chrome = p
				break
			}
		}
	}
	if chrome == "" {
		t.Fatal("no chromium on PATH; set CHROMIUM")
	}
	out, err := filepath.Abs("../../docs/images")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}

	status := &fakeStatus{m: map[int64]printerstatus.Status{}}
	filaments := &fakeFilaments{opts: sampleFilaments()}
	h := newHarnessDeps(t, func(d *Deps) {
		d.PrinterStatus = status
		d.Filaments = filaments
	})
	ids := seedScreenshots(t, h, status)

	shots := []struct {
		name, path string
		w, h       int
	}{
		{"board.png", "/", 1440, 1000},
		{"request.png", fmt.Sprintf("/requests/%d", ids.detail), 1280, 1000},
		{"new.png", "/requests/new", 1280, 1100},
		{"board-phone.png", "/", 390, 844},
	}
	for _, s := range shots {
		profile := t.TempDir()
		cmd := exec.Command(chrome,
			"--headless", "--disable-gpu", "--no-first-run", "--hide-scrollbars",
			"--user-data-dir="+profile,
			"--force-device-scale-factor=2",
			"--virtual-time-budget=4000",
			fmt.Sprintf("--window-size=%d,%d", s.w, s.h),
			"--screenshot="+filepath.Join(out, s.name),
			h.srv.URL+s.path,
		)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", s.name, err, b)
		}
		t.Logf("wrote %s", filepath.Join(out, s.name))
	}
}

type screenshotIDs struct{ detail int64 }

func seedScreenshots(t *testing.T, h *harness, status *fakeStatus) screenshotIDs {
	t.Helper()
	ctx := context.Background()
	st := h.store

	// The owner is the DEV_USER; make it exist with a friendly name.
	owner, err := st.EnsureDevUser(ctx, h.s.Config.DevUser)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `update users set name = 'Sam' where id = $1`, owner.ID); err != nil {
		t.Fatal(err)
	}

	printers := []*core.Printer{
		{Name: "Voron 2.4", BedX: 350, BedY: 350, BedZ: 340, Enclosed: true, Active: true, MoonrakerURL: "http://voron.invalid:7125"},
		{Name: "Bambu X1C", BedX: 256, BedY: 256, BedZ: 256, Enclosed: true, MultiMaterial: true, Active: true, MoonrakerURL: "http://x1c.invalid:7125"},
		{Name: "Prusa MK4", BedX: 250, BedY: 210, BedZ: 220, Active: true, MoonrakerURL: "http://mk4.invalid:7125"},
	}
	for _, p := range printers {
		if err := st.SavePrinter(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	ps, err := st.ListPrinters(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pid := map[string]int64{}
	for _, p := range ps {
		pid[p.Name] = p.ID
	}
	now := time.Now()
	status.m[pid["Voron 2.4"]] = printerstatus.Status{State: printerstatus.Printing, Job: "desk_organizer_v3", Progress: 0.62, Remaining: 83 * time.Minute, Layer: 148, Layers: 240, At: now}
	status.m[pid["Bambu X1C"]] = printerstatus.Status{State: printerstatus.Finished, Job: "dice_tower", Progress: 1, At: now, Since: now.Add(-2 * time.Hour)}
	status.m[pid["Prusa MK4"]] = printerstatus.Status{State: printerstatus.Printing, Job: "phone_stand", Progress: 0.18, Remaining: 47 * time.Minute, Layer: 31, Layers: 172, At: now}

	friends := map[string]*core.User{}
	for _, n := range []string{"Avery", "Jordan", "Riley", "Morgan", "Casey"} {
		friends[n] = h.friend(n)
	}

	day := func(n int) *time.Time {
		d := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, n)
		return &d
	}
	ip := func(n int) *int { return &n }
	p := func(name string) *int64 { id := pid[name]; return &id }
	fil := func(id int, label, material string, hexes ...string) core.ColorChoice {
		return core.ColorChoice{FilamentID: &id, Label: label, Material: material, Hexes: hexes}
	}
	str := func(s string) *string { return &s }

	reqs := []struct {
		who string
		r   core.Request
	}{
		{"Avery", core.Request{Title: "Desk cable organizer", Status: core.StatusPrinting, Quantity: 2, Printed: 1, PrinterID: p("Voron 2.4"),
			Material: "PLA", Colors: []core.ColorChoice{fil(2, "Prusament Galaxy Black", "PLA", "#1a1a1a")}, Timing: core.TimingFirm, NeededBy: day(2),
			SizeX: ip(180), SizeY: ip(60), SizeZ: ip(45), SourceURL: str("https://example.com/models/desk-cable-organizer"), SourceTitle: str("Desk cable organizer by example"),
			Notes: "Two please, one for home and one for the office. The black would be perfect.", Project: "Office refresh"}},
		{"Jordan", core.Request{Title: "Dice tower", Status: core.StatusReady, Quantity: 1, Printed: 1, PrinterID: p("Bambu X1C"),
			Material: "PLA", Colors: []core.ColorChoice{fil(5, "Eryone Rainbow", "PLA", "#ff0000", "#00ff00")}, Timing: core.TimingFlexible, NeededBy: day(5)}},
		{"Riley", core.Request{Title: "Planter with drainage tray", Status: core.StatusRequested, Quantity: 3,
			Material: "PETG", Colors: []core.ColorChoice{{Rough: "green", Label: "Green"}}, Timing: core.TimingWhenever,
			SizeX: ip(140), SizeY: ip(140), SizeZ: ip(120)}},
		{"Morgan", core.Request{Title: "Camera mount for the bike", Status: core.StatusRequested, Quantity: 1,
			Material: "ASA", Colors: []core.ColorChoice{fil(3, "Polymaker ASA Grey", "ASA", "#888888")}, NeedsEnclosure: true, Timing: core.TimingFirm, NeededBy: day(9)}},
		{"Casey", core.Request{Title: "Board game insert", Status: core.StatusRequested, Quantity: 1,
			Material: "PLA", Colors: []core.ColorChoice{fil(4, "Polymaker Sky Blue", "PLA", "#3070e0"), fil(2, "Prusament Galaxy Black", "PLA", "#1a1a1a")}, NeedsMultiMaterial: true,
			Timing: core.TimingFlexible, NeededBy: day(20), SizeX: ip(280), SizeY: ip(280), SizeZ: ip(70)}},
		{"Avery", core.Request{Title: "Monitor riser feet", Status: core.StatusRequested, Quantity: 4,
			Material: "PETG", Timing: core.TimingWhenever, Project: "Office refresh"}},
		{"Jordan", core.Request{Title: "Replacement knob for the stove", Status: core.StatusNeedsInfo, Quantity: 1,
			Timing: core.TimingFlexible, Notes: "Not sure of the shaft size yet."}},
		{"Riley", core.Request{Title: "Headphone hook", Status: core.StatusReady, Quantity: 1, Printed: 1, PrinterID: p("Prusa MK4"),
			Material: "PLA", Colors: []core.ColorChoice{fil(4, "Polymaker Sky Blue", "PLA", "#3070e0")}, Timing: core.TimingWhenever}},
		{"Morgan", core.Request{Title: "Phone stand", Status: core.StatusPrinting, Quantity: 1, PrinterID: p("Prusa MK4"),
			Material: "PLA", Colors: []core.ColorChoice{{Rough: "white", Label: "White"}}, Timing: core.TimingWhenever}},
	}
	var detail int64
	for i := range reqs {
		r := &reqs[i].r
		u := friends[reqs[i].who]
		r.RequesterID, r.CreatedByID = u.ID, u.ID
		if err := st.CreateRequest(ctx, r); err != nil {
			t.Fatalf("%s: %v", r.Title, err)
		}
		if i == 0 {
			detail = r.ID
		}
	}
	for _, e := range []struct {
		who  int64
		body string
	}{
		{friends["Avery"].ID, "Any chance these could be done by Friday?"},
		{owner.ID, "Should be! First one is off the printer, second is on now."},
	} {
		if err := st.AddEvent(ctx, detail, e.who, core.EventComment, e.body); err != nil {
			t.Fatal(err)
		}
	}
	return screenshotIDs{detail: detail}
}
