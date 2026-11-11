package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bjschafer/print-dis/internal/core"
)

// formErrors collects user-facing validation messages.
type formErrors []string

func (e *formErrors) add(format string, a ...any) { *e = append(*e, fmt.Sprintf(format, a...)) }

func trimmed(r *http.Request, name string) string { return strings.TrimSpace(r.PostFormValue(name)) }

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func checkbox(r *http.Request, name string) bool {
	v := r.PostFormValue(name)
	return v == "on" || v == "true" || v == "1"
}

func optPositiveInt(r *http.Request, name, label string, errs *formErrors) *int {
	s := trimmed(r, name)
	if s == "" {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		errs.add("%s must be a whole number above zero.", label)
		return nil
	}
	return &n
}

func optID(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

const maxTitle = 200

// applyRequestForm copies form fields onto req. Admin-only fields are applied
// only when admin is true; requesters cannot set them even by crafting a form.
func applyRequestForm(r *http.Request, req *core.Request, admin bool) formErrors {
	var errs formErrors

	req.Title = trimmed(r, "title")
	if req.Title == "" {
		errs.add("Give the request a title.")
	} else if len([]rune(req.Title)) > maxTitle {
		errs.add("The title is too long (at most %d characters).", maxTitle)
	}

	if q := trimmed(r, "quantity"); q == "" {
		req.Quantity = 1
	} else if n, err := strconv.Atoi(q); err != nil || n <= 0 || n > 10000 {
		errs.add("Quantity must be a whole number from 1 to 10000.")
	} else {
		req.Quantity = n
	}

	if d := trimmed(r, "needed_by"); d == "" {
		req.NeededBy = nil
	} else if t, err := time.Parse(time.DateOnly, d); err != nil {
		errs.add("Needed-by must be a date.")
	} else {
		req.NeededBy = &t
	}

	req.Timing = core.Timing(trimmed(r, "timing"))
	if !req.Timing.Valid() {
		req.Timing = core.TimingWhenever
	}
	req.Notes = strings.TrimSpace(strings.ReplaceAll(r.PostFormValue("notes"), "\r\n", "\n"))

	if raw := trimmed(r, "source_url"); raw == "" {
		req.SourceURL, req.SourceTitle = nil, nil
	} else if u, err := url.Parse(raw); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs.add("The link must be an http or https URL.")
	} else {
		req.SourceURL = &raw
	}
	if t := trimmed(r, "source_title"); t != "" {
		req.SourceTitle = &t
	}

	if r.PostForm.Has("material") {
		req.Material = strings.ToUpper(trimmed(r, "material"))
	}
	req.ColorNote = trimmed(r, "color_note")
	// spoolman_filament_id and the filament snapshot are handled by
	// applyFilament, which never trusts a client-supplied label or colour.

	req.NeedsMultiMaterial = checkbox(r, "needs_multi_material")
	req.NeedsEnclosure = checkbox(r, "needs_enclosure")
	req.SizeX = optPositiveInt(r, "size_x", "Width", &errs)
	req.SizeY = optPositiveInt(r, "size_y", "Depth", &errs)
	req.SizeZ = optPositiveInt(r, "size_z", "Height", &errs)
	if n := countSet(req.SizeX, req.SizeY, req.SizeZ); n != 0 && n != 3 {
		errs.add("Give all three dimensions, or none.")
	}

	if admin {
		if id, ok := optID(trimmed(r, "requester_id")); ok {
			req.RequesterID = id
		}
		if id, ok := optID(trimmed(r, "printer_id")); ok {
			req.PrinterID = &id
		} else if r.PostForm.Has("printer_id") {
			req.PrinterID = nil
		}
		if p := trimmed(r, "printed_count"); p != "" {
			if n, err := strconv.Atoi(p); err != nil || n < 0 {
				errs.add("Printed must be zero or more.")
			} else {
				req.Printed = n
			}
		}
	}
	return errs
}

func countSet(ps ...*int) int {
	n := 0
	for _, p := range ps {
		if p != nil {
			n++
		}
	}
	return n
}

// normHex returns "#rrggbb" for a valid hex colour, or "".
func normHex(s string) string {
	s = strings.TrimPrefix(strings.ToLower(s), "#")
	if len(s) != 6 && len(s) != 8 {
		return ""
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return ""
		}
	}
	return "#" + s[:6]
}

func applyPrinterForm(r *http.Request, p *core.Printer) formErrors {
	var errs formErrors
	p.Name = trimmed(r, "name")
	if p.Name == "" {
		errs.add("Give the printer a name.")
	}
	dim := func(name, label string) int {
		n, err := strconv.Atoi(trimmed(r, name))
		if err != nil || n <= 0 || n > 5000 {
			errs.add("%s must be a whole number of millimetres.", label)
			return 0
		}
		return n
	}
	p.BedX = dim("bed_x", "Bed width")
	p.BedY = dim("bed_y", "Bed depth")
	p.BedZ = dim("bed_z", "Bed height")
	p.MultiMaterial = checkbox(r, "multi_material")
	p.Enclosed = checkbox(r, "enclosed")
	p.Active = checkbox(r, "active")
	p.Notes = trimmed(r, "notes")

	// The source is a choice of one; only the chosen one's field is kept.
	// A form without the choice (an old page, a script) infers it.
	src := trimmed(r, "status_source")
	moon := strings.TrimRight(trimmed(r, "moonraker_url"), "/")
	bambu := trimmed(r, "bambuddy_printer_id")
	if !r.PostForm.Has("status_source") {
		switch {
		case moon != "" && bambu != "":
			errs.add("Choose either Moonraker or Bambuddy for live status, not both.")
		case moon != "":
			src = "moonraker"
		case bambu != "":
			src = "bambuddy"
		}
	}
	p.MoonrakerURL, p.BambuddyPrinterID, p.FormSource = "", nil, src
	switch src {
	case "":
	case "moonraker":
		p.MoonrakerURL = moon
		if u, err := url.Parse(moon); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs.add("The Moonraker address must be a full http:// or https:// address, like http://trident.lan:7125.")
		}
	case "bambuddy":
		n, err := strconv.Atoi(bambu)
		if err != nil || n <= 0 {
			errs.add("Choose which Bambuddy printer this is.")
		} else {
			p.BambuddyPrinterID = &n
		}
	default:
		errs.add("Choose a live status source: none, Moonraker or Bambuddy.")
	}
	return errs
}

// applyBambuddyForm reads the Bambuddy settings form into set.
func applyBambuddyForm(r *http.Request, set *core.Settings) formErrors {
	var errs formErrors
	raw := strings.TrimRight(trimmed(r, "bambuddy_url"), "/")
	// People paste the API address; the origin is what is wanted.
	raw = strings.TrimRight(strings.TrimSuffix(raw, "/api/v1"), "/")
	set.BambuddyURL = raw
	if raw != "" {
		if u, err := url.Parse(raw); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs.add("The Bambuddy address must be a full http:// or https:// address, like http://bambuddy.lan:8000.")
		}
	}
	switch key := trimmed(r, "bambuddy_api_key"); {
	case checkbox(r, "clear_api_key"):
		set.BambuddyAPIKey = ""
	case key != "":
		set.BambuddyAPIKey = key
	}
	return errs
}
