package web

import (
	"fmt"
	"html/template"
	"net/url"
	"strconv"
	"strings"
	"time"

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
	// swatch returns a safe CSS colour for inline style, or "".
	"swatch": func(p *string) template.CSS {
		if p == nil {
			return ""
		}
		if h := normHex(*p); h != "" {
			return template.CSS(h) // #nosec G203 -- validated hex
		}
		return ""
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
	// initial is the first letter of a name, upper-cased, for avatars.
	"initial": func(name string) string {
		for _, c := range name {
			return strings.ToUpper(string(c))
		}
		return "?"
	},
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
