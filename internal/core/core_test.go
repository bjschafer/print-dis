package core

import (
	"slices"
	"strings"
	"testing"
	"time"
)

var (
	h2d     = Printer{ID: 1, Name: "H2D", BedX: 325, BedY: 320, BedZ: 325, MultiMaterial: true, Enclosed: true, Active: true}
	trident = Printer{ID: 2, Name: "Trident", BedX: 350, BedY: 350, BedZ: 300, Enclosed: true, Active: true}
	pandora = Printer{ID: 3, Name: "Pandora", BedX: 220, BedY: 120, BedZ: 150, Active: true}
	retired = Printer{ID: 4, Name: "Old", BedX: 500, BedY: 500, BedZ: 500, MultiMaterial: true, Enclosed: true, Active: false}
	fleet   = []Printer{h2d, trident, pandora, retired}
)

func names(ps []Printer) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

func TestEligible(t *testing.T) {
	tests := []struct {
		name string
		req  Request
		want []string
	}{
		{"no requirements", Request{}, []string{"H2D", "Trident", "Pandora"}},
		{"multi-material", Request{NeedsMultiMaterial: true}, []string{"H2D"}},
		{"enclosure", Request{NeedsEnclosure: true}, []string{"H2D", "Trident"}},
		{"both", Request{NeedsMultiMaterial: true, NeedsEnclosure: true}, []string{"H2D"}},
		{"fits small bed", Request{SizeX: new(200), SizeY: new(100), SizeZ: new(100)}, []string{"H2D", "Trident", "Pandora"}},
		{"fits only rotated", Request{SizeX: new(110), SizeY: new(210), SizeZ: new(100)}, []string{"H2D", "Trident", "Pandora"}},
		{"too tall", Request{SizeX: new(100), SizeY: new(100), SizeZ: new(310)}, []string{"H2D"}},
		{"z is not swapped", Request{SizeX: new(200), SizeY: new(100), SizeZ: new(160)}, []string{"H2D", "Trident"}},
		{"exact fit", Request{SizeX: new(350), SizeY: new(350), SizeZ: new(300)}, []string{"Trident"}},
		{"too big for all", Request{SizeX: new(400), SizeY: new(100), SizeZ: new(100)}, nil},
		{"partial size ignored", Request{SizeX: new(9999)}, []string{"H2D", "Trident", "Pandora"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(Eligible(&tt.req, fleet))
			if !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckReasons(t *testing.T) {
	r := Request{NeedsMultiMaterial: true, NeedsEnclosure: true, SizeX: new(300), SizeY: new(300), SizeZ: new(200)}
	checks := Check(&r, fleet)
	want := map[string][]string{
		"H2D":     nil,
		"Trident": {"no multi-material"},
		"Pandora": {"no multi-material", "not enclosed", "does not fit"},
		"Old":     {"inactive"},
	}
	for _, c := range checks {
		w := want[c.Printer.Name]
		if c.Eligible != (len(w) == 0) {
			t.Errorf("%s eligible=%v", c.Printer.Name, c.Eligible)
		}
		if len(c.Reasons) != len(w) {
			t.Errorf("%s reasons %v, want %v", c.Printer.Name, c.Reasons, w)
			continue
		}
		for i := range w {
			if !strings.Contains(c.Reasons[i], w[i]) {
				t.Errorf("%s reason %d = %q, want containing %q", c.Printer.Name, i, c.Reasons[i], w[i])
			}
		}
	}
}

func TestMaterialWantsEnclosure(t *testing.T) {
	for m, want := range map[string]bool{
		"ABS": true, "abs": true, " ASA ": true, "PC": true, "PA": true, "PA12-CF": true, "ABS-GF": true, "PC+ABS": true,
		"PETG": false, "PLA": false, "": false, "PACKAGE": false, "PCTG": false, "PET": false,
	} {
		if got := MaterialWantsEnclosure(m); got != want {
			t.Errorf("%q = %v, want %v", m, got, want)
		}
	}
}

func TestUrgent(t *testing.T) {
	today := time.Date(2026, 11, 11, 15, 0, 0, 0, time.UTC)
	d := func(days int) *time.Time {
		x := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC).AddDate(0, 0, days)
		return &x
	}
	tests := []struct {
		name string
		r    Request
		want bool
	}{
		{"firm overdue", Request{Timing: TimingFirm, NeededBy: d(-2), Status: StatusPrinting}, true},
		{"firm today", Request{Timing: TimingFirm, NeededBy: d(0)}, true},
		{"firm in 3 days", Request{Timing: TimingFirm, NeededBy: d(3)}, true},
		{"firm in 4 days", Request{Timing: TimingFirm, NeededBy: d(4)}, false},
		{"flexible overdue", Request{Timing: TimingFlexible, NeededBy: d(-2)}, false},
		{"firm no date", Request{Timing: TimingFirm}, false},
		{"firm delivered", Request{Timing: TimingFirm, NeededBy: d(-2), Status: StatusDelivered}, false},
	}
	for _, tt := range tests {
		if got := tt.r.Urgent(today); got != tt.want {
			t.Errorf("%s: got %v", tt.name, got)
		}
	}
}

// TestPermissions walks every row of the table in design section 2.
func TestPermissions(t *testing.T) {
	admin := &User{ID: 1, IsAdmin: true}
	alice := &User{ID: 2}
	bob := &User{ID: 3}

	type verdicts struct{ view, edit, comment, cancel, upload bool }
	tests := []struct {
		status Status
		admin  verdicts
		owner  verdicts
		other  verdicts
	}{
		{StatusRequested, verdicts{true, true, true, true, true}, verdicts{true, true, true, true, true}, verdicts{}},
		{StatusNeedsInfo, verdicts{true, true, true, true, true}, verdicts{true, true, true, true, true}, verdicts{}},
		{StatusPrinting, verdicts{true, true, true, true, true}, verdicts{true, false, true, false, false}, verdicts{}},
		{StatusReady, verdicts{true, true, true, true, true}, verdicts{true, false, true, false, false}, verdicts{}},
		{StatusDelivered, verdicts{true, true, true, true, true}, verdicts{true, false, true, false, false}, verdicts{}},
		{StatusCancelled, verdicts{true, true, true, false, true}, verdicts{true, false, true, false, false}, verdicts{}},
	}
	for _, tt := range tests {
		r := &Request{ID: 10, RequesterID: alice.ID, Status: tt.status}
		for _, who := range []struct {
			name string
			u    *User
			want verdicts
		}{{"admin", admin, tt.admin}, {"owner", alice, tt.owner}, {"other", bob, tt.other}, {"nil", nil, verdicts{}}} {
			got := verdicts{CanView(who.u, r), CanEdit(who.u, r), CanComment(who.u, r), CanCancel(who.u, r), CanUploadFile(who.u, r)}
			if got != who.want {
				t.Errorf("%s/%s: got %+v, want %+v", tt.status, who.name, got, who.want)
			}
			if CanDownloadFile(who.u, r) != who.want.view {
				t.Errorf("%s/%s: download != view", tt.status, who.name)
			}
		}
	}

	for _, fn := range []struct {
		name string
		f    func(*User) bool
	}{{"move", CanMove}, {"admin fields", CanEditAdminFields}, {"file for", CanFileFor}, {"printers", CanManagePrinters}, {"people", CanManagePeople}, {"see all", CanSeeAll}, {"printer status", CanSeePrinterStatus}} {
		if !fn.f(admin) || fn.f(alice) || fn.f(nil) {
			t.Errorf("%s: only admins", fn.name)
		}
	}
}

func TestCanSetStatus(t *testing.T) {
	admin := &User{ID: 1, IsAdmin: true}
	alice := &User{ID: 2}
	bob := &User{ID: 3}
	for _, from := range Statuses {
		for _, to := range Statuses {
			r := &Request{RequesterID: alice.ID, Status: from}
			wantAdmin := from != to
			wantOwner := to == StatusCancelled && (from == StatusRequested || from == StatusNeedsInfo)
			if got := CanSetStatus(admin, r, to); got != wantAdmin {
				t.Errorf("admin %s->%s = %v", from, to, got)
			}
			if got := CanSetStatus(alice, r, to); got != wantOwner {
				t.Errorf("owner %s->%s = %v", from, to, got)
			}
			if CanSetStatus(bob, r, to) {
				t.Errorf("other %s->%s allowed", from, to)
			}
		}
	}
	if CanSetStatus(admin, &Request{Status: StatusRequested}, "bogus") {
		t.Error("invalid status allowed")
	}
}

func TestCanDeleteFile(t *testing.T) {
	admin := &User{ID: 1, IsAdmin: true}
	alice := &User{ID: 2}
	open := &Request{ID: 10, RequesterID: alice.ID, Status: StatusRequested}
	printing := &Request{ID: 10, RequesterID: alice.ID, Status: StatusPrinting}
	hers := &File{RequestID: 10, UploadedByID: alice.ID}
	adminsFile := &File{RequestID: 10, UploadedByID: admin.ID}
	other := &File{RequestID: 99, UploadedByID: alice.ID}

	tests := []struct {
		name string
		u    *User
		r    *Request
		f    *File
		want bool
	}{
		{"admin any", admin, printing, hers, true},
		{"owner own upload while open", alice, open, hers, true},
		{"owner own upload while printing", alice, printing, hers, false},
		{"owner admin's upload", alice, open, adminsFile, false},
		{"file on another request", alice, open, other, false},
		{"nil user", nil, open, hers, false},
	}
	for _, tt := range tests {
		if got := CanDeleteFile(tt.u, tt.r, tt.f); got != tt.want {
			t.Errorf("%s: got %v", tt.name, got)
		}
	}
}

func TestPrinterSource(t *testing.T) {
	id := 3
	for _, tt := range []struct {
		p    Printer
		want string
	}{
		{Printer{}, ""},
		{Printer{MoonrakerURL: "http://x"}, "moonraker"},
		{Printer{BambuddyPrinterID: &id}, "bambuddy"},
		{Printer{MoonrakerURL: "http://x", FormSource: "bambuddy"}, "bambuddy"},
	} {
		if got := tt.p.Source(); got != tt.want {
			t.Errorf("%+v: %q, want %q", tt.p, got, tt.want)
		}
	}
}
