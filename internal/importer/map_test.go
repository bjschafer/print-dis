package importer

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bjschafer/print-dis/internal/core"
)

var runDate = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)

const hdr = "Status,Item / model,Qty,Printed,Method,Material / color,Model / file link,Needed by,Timing,Request details"

// one maps a single data row under the standard header.
func one(t *testing.T, line string) Row {
	t.Helper()
	recs, err := ReadCSV(strings.NewReader(hdr + "\n" + line + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Map(recs, runDate)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(res.Rows))
	}
	return res.Rows[0]
}

func TestStatus(t *testing.T) {
	tests := []struct {
		in   string
		want core.Status
		skip string
	}{
		{"Requested", core.StatusRequested, ""},
		{"Needs info", core.StatusNeedsInfo, ""},
		{"needs  INFO", core.StatusNeedsInfo, ""},
		{"Printing", core.StatusPrinting, ""},
		{"Ready for pickup", core.StatusReady, ""},
		{"Ready", core.StatusReady, ""},
		{"Delivered", core.StatusDelivered, ""},
		{"Cancelled", core.StatusCancelled, ""},
		{"Canceled", core.StatusCancelled, ""},
		{"Maybe", "", `status not mapped: "Maybe"`},
		{"", "", "status not mapped: blank"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			r := one(t, tc.in+",Thing,,,,,,,,")
			if r.Skip != tc.skip || r.Req.Status != tc.want {
				t.Errorf("skip=%q status=%q, want skip=%q status=%q", r.Skip, r.Req.Status, tc.skip, tc.want)
			}
		})
	}
}

func TestFields(t *testing.T) {
	url := "https://www.printables.com/model/1"
	d := func(y int, m time.Month, day int) *time.Time {
		x := time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
		return &x
	}
	tests := []struct {
		name   string
		line   string
		qty    int
		printd int
		mat    string
		color  string
		encl   bool
		link   *string
		needed *time.Time
		timing core.Timing
		notes  string
	}{
		{name: "minimal", line: "Requested,Benchy,,,,,,,,", qty: 1, timing: "whenever"},
		{name: "qty int", line: "Requested,X,4,,,,,,,", qty: 4, timing: "whenever"},
		{name: "qty text", line: `Requested,X,2 plates,,,,,,,`, qty: 1, timing: "whenever", notes: "Qty: 2 plates"},
		{name: "qty 1 of Ea", line: `Requested,X,1 of Ea,,,,,,,`, qty: 1, timing: "whenever", notes: "Qty: 1 of Ea"},
		{name: "qty zero", line: `Requested,X,0,,,,,,,`, qty: 1, timing: "whenever", notes: "Qty: 0"},
		{name: "printed", line: "Printing,X,5,3,,,,,,", qty: 5, printd: 3, timing: "whenever"},
		{name: "printed negative", line: "Printing,X,5,-2,,,,,,", qty: 5, timing: "whenever"},
		{name: "printed text", line: "Printing,X,5,some,,,,,,", qty: 5, timing: "whenever", notes: "Printed: some"},
		{name: "resin", line: "Requested,X,,,RESIN,,,,,", qty: 1, timing: "whenever", notes: "Method: Resin"},
		{name: "fdm", line: "Requested,X,,,FDM,,,,,", qty: 1, timing: "whenever"},
		{name: "mat+color", line: "Requested,X,,,,PETG black,,,,", qty: 1, mat: "PETG", color: "black", timing: "whenever"},
		{name: "mat dash any", line: "Requested,X,,,,PLA - any,,,,", qty: 1, mat: "PLA", color: "any", timing: "whenever"},
		{name: "mat slash", line: "Requested,X,,,,ABS / orange,,,,", qty: 1, mat: "ABS", color: "orange", encl: true, timing: "whenever"},
		{name: "mat lower", line: "Requested,X,,,,petg,,,,", qty: 1, mat: "PETG", timing: "whenever"},
		{name: "mat plus", line: `Requested,X,,,,"PLA+, silk gold",,,,`, qty: 1, mat: "PLA+", color: "silk gold", timing: "whenever"},
		{name: "mat suffix", line: "Requested,X,,,,PETG-CF,,,,", qty: 1, mat: "PETG-CF", timing: "whenever"},
		{name: "mat PA12-CF", line: "Requested,X,,,,PA12-CF black,,,,", qty: 1, mat: "PA12-CF", color: "black", encl: true, timing: "whenever"},
		{name: "mat PCTG not enclosure", line: "Requested,X,,,,PCTG clear,,,,", qty: 1, mat: "PCTG", color: "clear", timing: "whenever"},
		{name: "mat PET vs PETG", line: "Requested,X,,,,PET grey,,,,", qty: 1, mat: "PET", color: "grey", timing: "whenever"},
		{name: "mat none", line: "Requested,X,,,,any,,,,", qty: 1, color: "any", timing: "whenever"},
		{name: "mat not a prefix", line: "Requested,X,,,,PLATINUM,,,,", qty: 1, color: "PLATINUM", timing: "whenever"},
		{name: "mat no separator", line: "Requested,X,,,,ASA,,,,", qty: 1, mat: "ASA", encl: true, timing: "whenever"},
		{name: "link url", line: "Requested,X,,,," + ",https://www.printables.com/model/1,,,", qty: 1, link: &url, timing: "whenever"},
		{name: "link text", line: "Requested,X,,,,,on my drive,,,", qty: 1, timing: "whenever", notes: "Link: on my drive"},
		{name: "link not http", line: "Requested,X,,,,,ftp://x/y,,,", qty: 1, timing: "whenever", notes: "Link: ftp://x/y"},
		{name: "date iso", line: "Requested,X,,,,,,2026-03-05,,", qty: 1, needed: d(2026, 3, 5), timing: "whenever"},
		{name: "date slash", line: "Requested,X,,,,,,3/5/2027,,", qty: 1, needed: d(2027, 3, 5), timing: "whenever"},
		{name: "date slash padded", line: "Requested,X,,,,,,03/05/2027,,", qty: 1, needed: d(2027, 3, 5), timing: "whenever"},
		{name: "date short month", line: `Requested,X,,,,,,"Jan 2, 2027",,`, qty: 1, needed: d(2027, 1, 2), timing: "whenever"},
		{name: "date long month", line: `Requested,X,,,,,,"January 2, 2027",,`, qty: 1, needed: d(2027, 1, 2), timing: "whenever"},
		{name: "date no year", line: "Requested,X,,,,,,12/25,,", qty: 1, needed: d(2026, 12, 25), timing: "whenever"},
		{name: "date garbage", line: "Requested,X,,,,,,soon-ish,,", qty: 1, timing: "whenever", notes: "Needed by: soon-ish"},
		{name: "date impossible", line: "Requested,X,,,,,,2/30,,", qty: 1, timing: "whenever", notes: "Needed by: 2/30"},
		{name: "timing firm", line: "Requested,X,,,,,,,FIRM,", qty: 1, timing: "firm"},
		{name: "timing flexible", line: "Requested,X,,,,,,,flexible,", qty: 1, timing: "flexible"},
		{name: "timing whenever", line: "Requested,X,,,,,,,Whenever,", qty: 1, timing: "whenever"},
		{name: "timing other", line: "Requested,X,,,,,,,ASAP,", qty: 1, timing: "whenever", notes: "Timing: ASAP"},
		{name: "details only", line: "Requested,X,,,,,,,,Please make it blue", qty: 1, timing: "whenever", notes: "Please make it blue"},
		{
			name: "all notes", line: `Requested,X,2 plates,lots,Resin,,somewhere,never,ASAP,"Line one
line two"`,
			qty: 1, timing: "whenever",
			notes: "Qty: 2 plates\nPrinted: lots\nMethod: Resin\nLink: somewhere\nNeeded by: never\nTiming: ASAP\n\nLine one\nline two",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := one(t, tc.line)
			if r.Skip != "" {
				t.Fatalf("skipped: %s", r.Skip)
			}
			q := r.Req
			if q.Quantity != tc.qty || q.Printed != tc.printd || q.Material != tc.mat || q.ColorNote != tc.color ||
				q.NeedsEnclosure != tc.encl || q.Timing != tc.timing || q.Notes != tc.notes {
				t.Errorf("got %+v", q)
			}
			if !reflect.DeepEqual(q.SourceURL, tc.link) {
				t.Errorf("link got %v want %v", q.SourceURL, tc.link)
			}
			if !reflect.DeepEqual(q.NeededBy, tc.needed) {
				t.Errorf("needed got %v want %v", q.NeededBy, tc.needed)
			}
		})
	}
}

func TestYearAssumptionReported(t *testing.T) {
	r := one(t, "Requested,X,,,,,,12/25,,")
	if len(r.Info) != 1 || !strings.Contains(r.Info[0], "assumed 2026") {
		t.Errorf("info = %v", r.Info)
	}
	if r := one(t, "Requested,X,,,,,,12/25/2026,,"); len(r.Info) != 0 {
		t.Errorf("info = %v", r.Info)
	}
}

func TestRowsAndHeaders(t *testing.T) {
	t.Run("blank and empty rows", func(t *testing.T) {
		recs, _ := ReadCSV(strings.NewReader(hdr + "\n,,,,,,,,,\n\nRequested,,2,,,,,,,\nRequested,Real,,,,,,,,\n"))
		res, err := Map(recs, runDate)
		if err != nil {
			t.Fatal(err)
		}
		// Fully empty rows vanish; the title-less one is reported with its record number. (encoding/csv drops bare blank lines, which Sheets never writes; it exports ",,,".)
		if len(res.Rows) != 2 {
			t.Fatalf("rows = %+v", res.Rows)
		}
		if res.Rows[0].Num != 3 || res.Rows[0].Skip != "no Item / model" {
			t.Errorf("row0 = %+v", res.Rows[0])
		}
		if res.Rows[1].Num != 4 || res.Rows[1].Skip != "" {
			t.Errorf("row1 = %+v", res.Rows[1])
		}
	})
	t.Run("BOM, reordered, extra and trailing empty columns, case", func(t *testing.T) {
		csv := "\ufeffnotes2, REQUEST DETAILS ,timing,needed by,model / file link,material / color,method,printed,qty,item / model,status,\n" +
			"x,detail,firm,2026-01-02,,PLA red,,1,3,Vase,Printing,\n"
		recs, err := ReadCSV(strings.NewReader(csv))
		if err != nil {
			t.Fatal(err)
		}
		res, err := Map(recs, runDate)
		if err != nil {
			t.Fatal(err)
		}
		q := res.Rows[0].Req
		if q.Title != "Vase" || q.Status != core.StatusPrinting || q.Quantity != 3 || q.Printed != 1 ||
			q.Material != "PLA" || q.ColorNote != "red" || q.Timing != core.TimingFirm || q.Notes != "detail" || q.NeededBy == nil {
			t.Errorf("got %+v", q)
		}
	})
	t.Run("short rows", func(t *testing.T) {
		r := one(t, "Requested,Short")
		if r.Skip != "" || r.Req.Title != "Short" || r.Req.Quantity != 1 {
			t.Errorf("got %+v", r)
		}
	})
	t.Run("missing column", func(t *testing.T) {
		recs, _ := ReadCSV(strings.NewReader("Status,Item / model,Qty\nRequested,X,1\n"))
		_, err := Map(recs, runDate)
		if err == nil || !strings.Contains(err.Error(), "Printed") || !strings.Contains(err.Error(), "Timing") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("empty file", func(t *testing.T) {
		if _, err := Map(nil, runDate); err == nil {
			t.Error("want error")
		}
	})
}

func TestKeys(t *testing.T) {
	keys := func(csv string) []string {
		recs, _ := ReadCSV(strings.NewReader(hdr + "\n" + csv))
		res, err := Map(recs, runDate)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range res.Rows {
			out = append(out, r.Key)
		}
		return out
	}
	a := keys("Requested,Vase,1,,,,,,,\nRequested,Cup,1,,,,,,,\n")
	b := keys("Requested,Cup,1,,,,,,,\n\nRequested,  VASE ,1,,,,,,,\n") // reordered, whitespace and case
	if a[0] != b[1] || a[1] != b[0] {
		t.Error("keys should not depend on position, case or spacing")
	}
	if c := keys("Requested,Vase,2,,,,,,,\n"); c[0] == a[0] {
		t.Error("edited row should change key")
	}
	d := keys("Requested,Vase,1,,,,,,,\nRequested,Vase,1,,,,,,,\n")
	if d[0] == d[1] || d[0] != a[0] {
		t.Error("identical rows need distinct keys, the first matching the single-row key")
	}
}
