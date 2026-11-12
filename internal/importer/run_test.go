package importer_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/importer"
	"github.com/bjschafer/print-dis/internal/store"
	"github.com/bjschafer/print-dis/internal/store/storetest"
)

const sheet = `Status,Item / model,Qty,Printed,Method,Material / color,Model / file link,Needed by,Timing,Request details
Requested,Benchy,2 plates,,FDM,PETG black,https://example.com/benchy,3/5,Firm,for my desk
Delivered,Vase,1,1,,,my drive,,,
Printing,Bracket,4,2,Resin,ABS / orange,,2026-12-01,Flexible,
Whatever,Odd status,1,,,,,,,
Requested,,1,,,,,,,
`

func mapSheet(t *testing.T) *importer.Result {
	t.Helper()
	recs, err := importer.ReadCSV(strings.NewReader(sheet))
	if err != nil {
		t.Fatal(err)
	}
	res, err := importer.Map(recs, time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func count(t *testing.T, st *store.Store, q string) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(), q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRun(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	const email = "Friend@Example.com"

	// Dry run writes nothing, not even the placeholder.
	var out bytes.Buffer
	res := mapSheet(t)
	sum, infos, err := importer.Run(ctx, st, res, email, "", true)
	if err != nil {
		t.Fatal(err)
	}
	importer.Report(&out, res, sum, infos, true)
	t.Log("\n" + out.String())
	if sum.Imported != 3 || sum.Skipped != 2 || sum.AlreadyImported != 0 {
		t.Errorf("dry summary = %+v", sum)
	}
	if !strings.Contains(out.String(), "would import") || !strings.Contains(out.String(), "skipped: no Item / model") {
		t.Errorf("report:\n%s", out.String())
	}
	if n := count(t, st, `select count(*) from requests`) + count(t, st, `select count(*) from users`); n != 0 {
		t.Fatalf("dry run wrote %d rows", n)
	}

	// Real run.
	res = mapSheet(t)
	sum, _, err = importer.Run(ctx, st, res, email, "Phii", false)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Imported != 3 || sum.Skipped != 2 {
		t.Errorf("summary = %+v", sum)
	}
	u, err := st.GetUserByEmail(ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	if u.Name != "Phii" || !u.Placeholder() {
		t.Errorf("user = %+v", u)
	}
	reqs, err := st.ListRequests(ctx, store.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 3 {
		t.Fatalf("got %d requests", len(reqs))
	}
	by := map[string]core.Request{}
	for _, r := range reqs {
		by[r.Title] = r
		if r.RequesterID != u.ID || r.CreatedByID != u.ID {
			t.Errorf("%s owned by %d/%d", r.Title, r.RequesterID, r.CreatedByID)
		}
	}
	b := by["Benchy"]
	if b.Quantity != 1 || b.Notes != "Qty: 2 plates\n\nfor my desk" || b.Material != "PETG" || len(b.Colors) != 1 || b.Colors[0].Rough != "black" ||
		b.Timing != core.TimingFirm || b.NeededBy == nil || b.NeededBy.Year() != 2026 || b.SourceURL == nil || b.Status != core.StatusRequested {
		t.Errorf("benchy = %+v", b)
	}
	if v := by["Vase"]; v.Notes != "Link: my drive" || v.Printed != 1 || v.Status != core.StatusDelivered {
		t.Errorf("vase = %+v", v)
	}
	if br := by["Bracket"]; br.Notes != "Method: Resin" || !br.NeedsEnclosure || br.Material != "ABS" || br.Printed != 2 {
		t.Errorf("bracket = %+v", br)
	}

	// Re-run adds nothing.
	res = mapSheet(t)
	sum, _, err = importer.Run(ctx, st, res, email, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Imported != 0 || sum.AlreadyImported != 3 {
		t.Errorf("rerun summary = %+v", sum)
	}
	if n := count(t, st, `select count(*) from requests`); n != 3 {
		t.Errorf("after rerun: %d requests", n)
	}
	if n := count(t, st, `select count(*) from users`); n != 1 {
		t.Errorf("after rerun: %d users", n)
	}
}

func TestRunRollsBack(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	res := mapSheet(t)
	// A title containing a NUL byte is rejected by Postgres, failing mid-batch.
	res.Rows[2].Req.Title = "bad\x00title"
	_, _, err := importer.Run(ctx, st, res, "x@example.com", "", false)
	if err == nil {
		t.Fatal("want error")
	}
	if errors.Is(err, store.ErrNotFound) {
		t.Fatalf("wrong error: %v", err)
	}
	if n := count(t, st, `select count(*) from requests`) + count(t, st, `select count(*) from users`); n != 0 {
		t.Errorf("rollback left %d rows", n)
	}
}

func TestRunBadRequester(t *testing.T) {
	st := storetest.New(t)
	if _, _, err := importer.Run(context.Background(), st, mapSheet(t), "not-an-email", "", true); err == nil {
		t.Error("want error")
	}
}
