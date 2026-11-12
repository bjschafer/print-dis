package web

import (
	"fmt"
	"hash/fnv"
	"html/template"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/bjschafer/print-dis/internal/core"
)

var funcs = template.FuncMap{
	"dict": func(kv ...any) (map[string]any, error) {
		if len(kv)%2 != 0 {
			return nil, fmt.Errorf("dict: odd number of arguments")
		}
		m := make(map[string]any, len(kv)/2)
		for i := 0; i < len(kv); i += 2 {
			k, ok := kv[i].(string)
			if !ok {
				return nil, fmt.Errorf("dict: key %v is not a string", kv[i])
			}
			m[k] = kv[i+1]
		}
		return m, nil
	},
	"newPrinter": func() core.Printer { return core.Printer{Active: true} },
	"str": func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	},
	"int": func(p *int) string {
		if p == nil {
			return ""
		}
		return strconv.Itoa(*p)
	},
	"deref": func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	},
	"id": func(p *int64) int64 {
		if p == nil {
			return 0
		}
		return *p
	},
	"date": func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.Format(time.DateOnly)
	},
	"shortDate": func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.Format("Jan 2")
	},
	"when": func(t time.Time) string { return t.Local().Format("Jan 2, 15:04") },
	"urgent": func(r core.Request, today time.Time) bool {
		return r.Urgent(today)
	},
	"statuses":       func() []core.Status { return core.Statuses },
	"boardStatuses":  func() []core.Status { return core.BoardStatuses },
	"timings":        func() []core.Timing { return core.Timings },
	"canEdit":        core.CanEdit,
	"canComment":     core.CanComment,
	"canSetStatus":   core.CanSetStatus,
	"canMove":        core.CanMove,
	"canCancel":      core.CanCancel,
	"isAdmin":        func(u *core.User) bool { return u != nil && u.IsAdmin },
	"wantsEnclosure": core.MaterialWantsEnclosure,
	"progress": func(r core.Request) string {
		if r.Quantity <= 1 && r.Printed == 0 {
			return ""
		}
		return fmt.Sprintf("%d/%d", r.Printed, r.Quantity)
	},
	// swatches is a CSS background for a swatch of these colors: the color
	// itself, or a pie of them for a multi-color filament. "" when none.
	"swatches": swatchCSS,
	// stripe is a CSS background for a card's edge: the colors of every
	// choice, top to bottom. "" when no choice has a color.
	"stripe": func(cs []core.ColorChoice) template.CSS {
		var hexes []string
		for _, c := range cs {
			hexes = append(hexes, c.Hexes...)
		}
		return bandsCSS("to bottom", hexes)
	},
	"list":        func(c core.ColorChoice) []core.ColorChoice { return []core.ColorChoice{c} },
	"roughColors": func() []core.RoughColor { return core.RoughColors },
	"maxColors":   func() int { return core.MaxColors },
	"hasPrefix":   strings.HasPrefix,
	"lower":       strings.ToLower,
	// grams formats filament left on the shelf: "340 g", "1.2 kg".
	"grams": func(g float64) string {
		if g >= 1000 {
			return fmt.Sprintf("%.1f kg", g/1000)
		}
		return fmt.Sprintf("%.0f g", g)
	},
	// sortLink keeps the current filters and toggles direction on the active column.
	"sortLink": func(q url.Values, col string) string {
		v := url.Values{}
		for k, vs := range q {
			v[k] = vs
		}
		if q.Get("sort") == col && q.Get("desc") == "" {
			v.Set("desc", "1")
		} else {
			v.Del("desc")
		}
		v.Set("sort", col)
		return "?" + v.Encode()
	},
	"sortMark": func(q url.Values, col string) string {
		if q.Get("sort") != col {
			return ""
		}
		if q.Get("desc") != "" {
			return " ▾"
		}
		return " ▴"
	},
	// pct is the printed share of a request, 0 to 100, for progress bars.
	"pct": func(r core.Request) int {
		if r.Quantity <= 0 {
			return 0
		}
		return min(100, max(0, r.Printed*100/r.Quantity))
	},
	"initial":   initials,
	"avatarHue": avatarHue,
	// left formats a remaining print time: "1h 20m left", "12m left".
	"left": func(d time.Duration) string {
		m := int(d.Round(time.Minute) / time.Minute)
		switch {
		case m < 1:
			return "under a minute left"
		case m < 60:
			return fmt.Sprintf("%dm left", m)
		}
		return fmt.Sprintf("%dh %02dm left", m/60, m%60)
	},
	"bytes": func(n int64) string {
		switch {
		case n >= 1<<20:
			return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
		case n >= 1<<10:
			return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
		}
		return fmt.Sprintf("%d B", n)
	},
}

// validHexes keeps the valid colors of hexes, normalized.
func validHexes(hexes []string) []string {
	var out []string
	for _, h := range hexes {
		if n := normHex(h); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func swatchCSS(hexes []string) template.CSS {
	hs := validHexes(hexes)
	switch len(hs) {
	case 0:
		return ""
	case 1:
		return template.CSS(hs[0]) // #nosec G203 -- validated hex
	}
	return bandsCSS("from 0deg", hs)
}

// bandsCSS is a hard-edged gradient with one equal band per color. dir is
// "to bottom" for a linear stripe or "from 0deg" for a pie.
func bandsCSS(dir string, hexes []string) template.CSS {
	hs := validHexes(hexes)
	switch len(hs) {
	case 0:
		return ""
	case 1:
		return template.CSS(hs[0]) // #nosec G203 -- validated hex
	}
	kind := "linear-gradient"
	if strings.HasPrefix(dir, "from") {
		kind = "conic-gradient"
	}
	var b strings.Builder
	b.WriteString(kind + "(" + dir)
	step := 100.0 / float64(len(hs))
	for i, h := range hs {
		fmt.Fprintf(&b, ", %s %.2f%% %.2f%%", h, step*float64(i), step*float64(i+1))
	}
	b.WriteString(")")
	return template.CSS(b.String()) // #nosec G203 -- validated hexes and fixed syntax
}

// initials are up to two letters for an avatar: the first letters of the
// first and last words of a name ("Ada Lovelace" is "AL"), or of the part of
// an email before the @.
func initials(name string) string {
	name, _, _ = strings.Cut(name, "@")
	words := strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	first := func(w string) string {
		for _, c := range w {
			return strings.ToUpper(string(c))
		}
		return ""
	}
	switch len(words) {
	case 0:
		return "?"
	case 1:
		return first(words[0])
	}
	return first(words[0]) + first(words[len(words)-1])
}

// avatarHue gives each person a stable hue for their avatar, so people are
// told apart at a glance. It is one of 24 steps of 15 degrees, chosen by a
// hash of the name, and goes in a CSS custom property.
func avatarHue(name string) template.CSS {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(name))))
	return template.CSS(fmt.Sprintf("--av-h: %d", (h.Sum32()%24)*15)) //nolint:gosec // G203: only a number is formatted in
}
