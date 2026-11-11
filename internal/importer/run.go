package importer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/store"
)

// Summary counts the outcome of a run.
type Summary struct {
	Imported        int // rows inserted, or that would be on a dry run
	AlreadyImported int
	Skipped         int // blank title, unmapped status
}

// Run marks rows already in the database, then (unless dryRun) inserts the
// rest for the requester in one transaction. It writes nothing on a dry run,
// including the placeholder user. res is updated so Report shows the outcome.
// placeholderName names the requester if they must be created; empty means
// the part of the email before the @.
func Run(ctx context.Context, st *store.Store, res *Result, email, placeholderName string, dryRun bool) (Summary, []string, error) {
	var sum Summary
	var infos []string
	email = strings.ToLower(strings.TrimSpace(email))
	name, _, ok := strings.Cut(email, "@")
	if !ok || name == "" {
		return sum, nil, fmt.Errorf("requester %q is not an email address", email)
	}
	if n := strings.TrimSpace(placeholderName); n != "" {
		name = n
	}
	if _, err := st.GetUserByEmail(ctx, email); err == nil {
		infos = append(infos, fmt.Sprintf("requester %s exists", email))
	} else if errors.Is(err, store.ErrNotFound) {
		if dryRun {
			infos = append(infos, fmt.Sprintf("requester %s does not exist; would create placeholder %q", email, name))
		} else {
			infos = append(infos, fmt.Sprintf("requester %s does not exist; creating placeholder %q", email, name))
		}
	} else {
		return sum, nil, fmt.Errorf("look up requester: %w", err)
	}

	var keys []string
	for _, r := range res.Rows {
		if r.Skip == "" {
			keys = append(keys, r.Key)
		}
	}
	existing, err := st.ExistingImportKeys(ctx, keys)
	if err != nil {
		return sum, nil, fmt.Errorf("check existing rows: %w", err)
	}
	var items []store.ImportItem
	for i := range res.Rows {
		r := &res.Rows[i]
		if r.Skip == "" && existing[r.Key] {
			r.Skip = "already imported"
			sum.AlreadyImported++
		}
		if r.Skip == "" {
			items = append(items, store.ImportItem{Key: r.Key, Req: r.Req})
		}
	}
	if !dryRun && len(items) > 0 {
		inserted, err := st.ImportRequests(ctx, email, name, items)
		if err != nil {
			return sum, nil, fmt.Errorf("import (rolled back): %w", err)
		}
		if len(inserted) != len(items) { // a concurrent import got there first
			got := map[string]bool{}
			for _, k := range inserted {
				got[k] = true
			}
			for i := range res.Rows {
				if r := &res.Rows[i]; r.Skip == "" && !got[r.Key] {
					r.Skip = "already imported"
					sum.AlreadyImported++
				}
			}
		}
	}
	for _, r := range res.Rows {
		switch {
		case r.Skip == "":
			sum.Imported++
		case r.Skip != "already imported":
			sum.Skipped++
		}
	}
	return sum, infos, nil
}

// Report writes one line per row and the totals.
func Report(w io.Writer, res *Result, sum Summary, infos []string, dryRun bool) {
	verb, total := "imported", "imported"
	if dryRun {
		verb, total = "would import", "would be imported"
	}
	for _, i := range infos {
		_, _ = fmt.Fprintf(w, "note: %s\n", i)
	}
	for _, r := range res.Rows {
		if r.Skip != "" {
			_, _ = fmt.Fprintf(w, "row %d: %q: skipped: %s\n", r.Num, r.Title, r.Skip)
			continue
		}
		_, _ = fmt.Fprintf(w, "row %d: %q: %s: %s\n", r.Num, r.Title, verb, summarize(&r.Req))
		for _, i := range r.Info {
			_, _ = fmt.Fprintf(w, "    note: %s\n", i)
		}
	}
	_, _ = fmt.Fprintf(w, "\n%d %s, %d already imported, %d skipped\n", sum.Imported, total, sum.AlreadyImported, sum.Skipped)
}

func summarize(r *core.Request) string {
	parts := []string{
		"status=" + r.Status.Label(),
		fmt.Sprintf("qty=%d", r.Quantity),
	}
	if r.Printed > 0 {
		parts = append(parts, fmt.Sprintf("printed=%d", r.Printed))
	}
	if r.Material != "" {
		parts = append(parts, "material="+r.Material)
	}
	if r.ColorNote != "" {
		parts = append(parts, fmt.Sprintf("color=%q", r.ColorNote))
	}
	if r.NeedsEnclosure {
		parts = append(parts, "enclosure")
	}
	if r.NeededBy != nil {
		parts = append(parts, "needed="+r.NeededBy.Format("2006-01-02"))
	}
	parts = append(parts, "timing="+string(r.Timing))
	if r.SourceURL != nil {
		parts = append(parts, "link")
	}
	if r.Notes != "" {
		n := strings.Join(strings.Fields(strings.ReplaceAll(r.Notes, "\n", " / ")), " ")
		if rs := []rune(n); len(rs) > 80 {
			n = string(rs[:77]) + "..."
		}
		parts = append(parts, fmt.Sprintf("notes=%q", n))
	}
	return strings.Join(parts, " ")
}
