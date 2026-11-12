package spoolman

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// Materials returns the distinct materials of opts, sorted.
func Materials(opts []FilamentOption) []string {
	seen := map[string]bool{}
	var out []string
	for _, o := range opts {
		if m := o.Material; m != "" && !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}

// Closest returns up to n in-stock filaments nearest in color to hex,
// nearest first. A non-empty material keeps only filaments whose material
// starts with it, so "PETG" matches "PETG-CF". Filaments without a known
// color are left out, as are any too far off to be a sensible match.
func Closest(opts []FilamentOption, hex, material string, n int) []FilamentOption {
	target, ok := rgb(hex)
	if !ok || n <= 0 {
		return nil
	}
	material = strings.ToUpper(strings.TrimSpace(material))
	type scored struct {
		o FilamentOption
		d float64
	}
	var cands []scored
	for _, o := range opts {
		if !o.InStock || (material != "" && !strings.HasPrefix(o.Material, material)) {
			continue
		}
		best := -1.0
		for _, h := range o.Hexes {
			if c, ok := rgb(h); ok {
				if d := distance(target, c); best < 0 || d < best {
					best = d
				}
			}
		}
		if best >= 0 && best <= maxMatchDistance {
			cands = append(cands, scored{o, best})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].d < cands[j].d })
	out := make([]FilamentOption, 0, min(n, len(cands)))
	for i := 0; i < len(cands) && i < n; i++ {
		out = append(out, cands[i].o)
	}
	return out
}

// maxMatchDistance is roughly "the same family of color": red to orange is
// in, red to blue is out.
const maxMatchDistance = 300

func rgb(hex string) ([3]float64, bool) {
	h := NormalizeHex(hex)
	if h == "" {
		return [3]float64{}, false
	}
	var c [3]float64
	for i := range 3 {
		v, err := strconv.ParseUint(h[1+2*i:3+2*i], 16, 8)
		if err != nil {
			return c, false
		}
		c[i] = float64(v)
	}
	return c, true
}

// distance is the "redmean" weighted RGB distance, a cheap approximation of
// how different two colors look.
func distance(a, b [3]float64) float64 {
	rm := (a[0] + b[0]) / 2
	dr, dg, db := a[0]-b[0], a[1]-b[1], a[2]-b[2]
	sq := (2+rm/256)*dr*dr + 4*dg*dg + (2+(255-rm)/256)*db*db
	return math.Sqrt(sq)
}
