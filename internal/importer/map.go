// Package importer loads a CSV export of the old Google Sheet. Mapping is pure
// (Map); database work is in Run.
package importer

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/bjschafer/print-dis/internal/core"
)

// Column names, in canonical order. Headers match case-insensitively and trimmed.
const (
	colStatus   = "Status"
	colItem     = "Item / model"
	colQty      = "Qty"
	colPrinted  = "Printed"
	colMethod   = "Method"
	colMaterial = "Material / color"
	colLink     = "Model / file link"
	colNeeded   = "Needed by"
	colTiming   = "Timing"
	// colPrintNotes is optional: the owner's own column, kept in the notes.
	colPrintNotes = "Print notes / blocker"
	colDetails    = "Request details"
)

var columns = []string{colStatus, colItem, colQty, colPrinted, colMethod, colMaterial, colLink, colNeeded, colTiming, colDetails}

// Row is the outcome for one sheet row.
type Row struct {
	Num   int    // sheet row number; the header is row 1
	Title string // as it appears in the sheet
	Key   string // stable identity, see rowKey
	Req   core.Request
	// Skip is non-empty when the row will not be imported, and says why.
	Skip string
	// Info are remarks about assumptions made while mapping (not stored).
	Info []string
}

// Result is the whole mapping of one file.
type Result struct {
	Rows []Row
}

// ReadCSV parses a Google Sheets CSV export, stripping a UTF-8 BOM.
func ReadCSV(r io.Reader) ([][]string, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	recs, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read csv: %w", err)
	}
	if len(recs) > 0 && len(recs[0]) > 0 {
		recs[0][0] = strings.TrimPrefix(recs[0][0], "\ufeff")
	}
	return recs, nil
}

// Map turns CSV records (first is the header) into requests. now supplies the
// year assumed for dates written without one. It fails only when the header
// lacks a required column.
func Map(records [][]string, now time.Time) (*Result, error) {
	if len(records) == 0 {
		return nil, errors.New("file is empty: no header row")
	}
	idx := map[string]int{}
	for i, h := range records[0] {
		h = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))
		if _, dup := idx[h]; !dup {
			idx[h] = i
		}
	}
	pos := map[string]int{}
	var missing []string
	for _, c := range columns {
		i, ok := idx[strings.ToLower(c)]
		if !ok {
			missing = append(missing, c)
			continue
		}
		pos[c] = i
	}
	if i, ok := idx[strings.ToLower(colPrintNotes)]; ok {
		pos[colPrintNotes] = i
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required column(s): %s", strings.Join(missing, ", "))
	}

	res := &Result{}
	seen := map[string]int{}
	for n, rec := range records[1:] {
		get := func(c string) string {
			if i, ok := pos[c]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		vals := make([]string, len(columns))
		empty := true
		for i, c := range columns {
			vals[i] = get(c)
			if vals[i] != "" {
				empty = false
			}
		}
		if empty {
			continue
		}
		row := mapRow(n+2, get, now)
		row.Key = rowKey(vals, seen)
		res.Rows = append(res.Rows, row)
	}
	return res, nil
}

// rowKey is the identity of a row across re-runs: a SHA-256 of the ten mapped
// columns, trimmed, lower-cased and with runs of whitespace collapsed. It does
// not depend on row position, so reordering or inserting rows is harmless, and
// not on the mapping rules, so improving them does not change keys. Rows with
// identical contents get a numbered suffix (first, second, ...) so genuine
// duplicates in the sheet are all imported once. Editing a row in the sheet
// changes its key: a re-import then adds the edited row as a new request, and
// never touches existing ones, so changes made in the app are never overwritten.
func rowKey(vals []string, seen map[string]int) string {
	norm := make([]string, len(vals))
	for i, v := range vals {
		norm[i] = strings.ToLower(strings.Join(strings.Fields(v), " "))
	}
	base := strings.Join(norm, "\x1f")
	seen[base]++
	sum := sha256.Sum256([]byte(fmt.Sprintf("print-dis/import-sheet/v1\x1e%s\x1e%d", base, seen[base])))
	return hex.EncodeToString(sum[:])
}

var statusNames = map[string]core.Status{
	"requested":        core.StatusRequested,
	"needs info":       core.StatusNeedsInfo,
	"printing":         core.StatusPrinting,
	"ready for pickup": core.StatusReady,
	"ready":            core.StatusReady,
	"delivered":        core.StatusDelivered,
	"cancelled":        core.StatusCancelled,
	"canceled":         core.StatusCancelled,
}

func mapRow(num int, get func(string) string, now time.Time) Row {
	row := Row{Num: num, Title: get(colItem)}
	r := &row.Req
	r.Title = row.Title

	statusText := get(colStatus)
	st, ok := statusNames[strings.ToLower(strings.Join(strings.Fields(statusText), " "))]
	switch {
	case row.Title == "":
		row.Skip = "no Item / model"
		return row
	case statusText == "":
		row.Skip = "status not mapped: blank"
		return row
	case !ok:
		row.Skip = fmt.Sprintf("status not mapped: %q", statusText)
		return row
	}
	r.Status = st

	var notes []string

	// Qty
	r.Quantity = 1
	if q := get(colQty); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n >= 1 {
			r.Quantity = n
		} else {
			notes = append(notes, "Qty: "+q)
		}
	}

	// Printed
	if p := get(colPrinted); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			r.Printed = max(n, 0)
		} else {
			notes = append(notes, "Printed: "+p)
		}
	}

	// Method
	if m := get(colMethod); strings.EqualFold(m, "resin") {
		notes = append(notes, "Method: Resin")
	}

	// Material / color
	var colorNote string
	r.Material, colorNote = ParseMaterial(get(colMaterial))
	r.Colors = core.ParseColorNote(colorNote)
	r.NeedsEnclosure = core.MaterialWantsEnclosure(r.Material)

	// Link
	if l := get(colLink); l != "" {
		if isHTTPURL(l) {
			r.SourceURL = &l
		} else {
			notes = append(notes, "Link: "+l)
		}
	}

	// Needed by
	if d := get(colNeeded); d != "" {
		t, yearless, ok := ParseDate(d, now.Year())
		if ok {
			r.NeededBy = &t
			if yearless {
				row.Info = append(row.Info, fmt.Sprintf("needed by %q has no year; assumed %d", d, now.Year()))
			}
		} else {
			notes = append(notes, "Needed by: "+d)
		}
	}

	// Timing
	r.Timing = core.TimingWhenever
	if t := get(colTiming); t != "" {
		// The sheet's values are phrases such as "Firm date" and
		// "Flexible (+few days)"; the first word decides.
		var first string
		if words := strings.FieldsFunc(t, func(c rune) bool { return !unicode.IsLetter(c) }); len(words) > 0 {
			first = strings.ToLower(words[0])
		}
		switch tt := core.Timing(first); tt {
		case core.TimingFirm, core.TimingFlexible, core.TimingWhenever:
			r.Timing = tt
		default:
			notes = append(notes, "Timing: "+t)
		}
	}

	// Optional column; not part of the row key.
	if pn := get(colPrintNotes); pn != "" {
		notes = append(notes, "Print notes: "+pn)
	}

	r.Notes = strings.Join(notes, "\n")
	if d := get(colDetails); d != "" {
		if r.Notes != "" {
			r.Notes += "\n\n"
		}
		r.Notes += d
	}
	return row
}

func isHTTPURL(s string) bool {
	if strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// A leading material: family, optional number (PA12), optional suffixes (+, -CF).
var materialRE = regexp.MustCompile(`(?i)^(PETG|PCTG|PLA|ABS|ASA|TPU|HIPS|PVA|PET|PC|PA)(\d{0,2})((?:\+|-(?:CF|GF|HF|HT|FR|ESD|LW))*)`)

const separators = " \t-/,;:|"

// A material written in capitals anywhere in the text: "Tinmorry blue PETG-GF".
var embeddedMaterialRE = regexp.MustCompile(`(?:^|[^A-Za-z0-9])((?:PETG|PCTG|PLA|ABS|ASA|TPU|HIPS|PVA|PET|PC|PA)\d{0,2}(?:\+|-(?:CF|GF|HF|HT|FR|ESD|LW))*)(?:$|[^A-Za-z0-9+])`)

// ParseMaterial splits free text such as "PETG black" into an upper-cased
// material and the remaining color note. A material elsewhere in the text,
// in capitals, is recognised too, and then the whole text is kept as the
// color note since vendor and color surround it. Unrecognised text is all
// color.
func ParseMaterial(s string) (material, colorNote string) {
	s = strings.TrimSpace(s)
	if m := materialRE.FindString(s); m != "" {
		rest := s[len(m):]
		if rest == "" || !isWordByte(rest[0]) {
			return strings.ToUpper(m), strings.Trim(rest, separators)
		}
	}
	if m := embeddedMaterialRE.FindStringSubmatch(s); m != nil {
		return m[1], s
	}
	return "", s
}

func isWordByte(b byte) bool {
	return b == '+' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

var dateLayouts = []string{"2006-01-02", "1/2/2006", "01/02/2006", "1/2/06", "Jan 2, 2006", "January 2, 2006"}

// ParseDate parses the date formats seen in the sheet. "1/2" has no year and
// gets defaultYear; yearless reports that.
func ParseDate(s string, defaultYear int) (t time.Time, yearless, ok bool) {
	s = strings.TrimSpace(s)
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, false, true
		}
	}
	// Parse "1/2" with a leap year so 2/29 is accepted, then validate in the target year.
	if t, err := time.Parse("2006 1/2", "2000 "+s); err == nil {
		d := time.Date(defaultYear, t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		if d.Month() == t.Month() {
			return d, true, true
		}
	}
	return time.Time{}, false, false
}
