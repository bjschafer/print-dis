package web

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"strconv"
	"sync"
)

// The web app manifest and its icons, so the site can be added to a phone's
// home screen and open without browser chrome.

// appIcon is one PNG the manifest or iOS asks for. scale shrinks the logo
// about the centre: a maskable icon keeps it inside the middle 80% circle
// that launchers may crop to.
type appIcon struct {
	path     string
	size     int
	scale    float64
	purpose  string
	manifest bool
}

var appIcons = []appIcon{
	{path: "/apple-touch-icon.png", size: 180, scale: 1},
	{path: "/icon-192.png", size: 192, scale: 1, purpose: "any", manifest: true},
	{path: "/icon-512.png", size: 512, scale: 1, purpose: "any", manifest: true},
	{path: "/icon-maskable-512.png", size: 512, scale: 0.8, purpose: "maskable", manifest: true},
}

// pwaAssets renders the icons once per process: the accent cannot change
// without a restart.
type pwaAssets struct {
	once  sync.Once
	icons map[string][]byte
}

func (s *Server) icon(path string) []byte {
	s.pwa.once.Do(func() {
		bg, fg := parseHex(s.accent()), parseHex(textOn(s.accent()))
		s.pwa.icons = make(map[string][]byte, len(appIcons))
		for _, ic := range appIcons {
			s.pwa.icons[ic.path] = renderIcon(ic.size, ic.scale, bg, fg)
		}
	})
	return s.pwa.icons[path]
}

func (s *Server) appIcon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(s.icon(r.URL.Path))
}

func (s *Server) manifest(w http.ResponseWriter, _ *http.Request) {
	type icon struct {
		Src     string `json:"src"`
		Sizes   string `json:"sizes"`
		Type    string `json:"type"`
		Purpose string `json:"purpose"`
	}
	var icons []icon
	for _, ic := range appIcons {
		if ic.manifest {
			n := strconv.Itoa(ic.size)
			icons = append(icons, icon{Src: ic.path, Sizes: n + "x" + n, Type: "image/png", Purpose: ic.purpose})
		}
	}
	// The colors are the dark theme's background, matching the dark
	// theme-color meta tag: the manifest cannot follow the OS setting.
	body, _ := json.Marshal(map[string]any{
		"name":             "print-dis",
		"short_name":       "print-dis",
		"id":               "/",
		"start_url":        "/",
		"scope":            "/",
		"display":          "standalone",
		"background_color": "#0f1115",
		"theme_color":      "#0f1115",
		"icons":            icons,
	})
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(body)
}

// logoStrokes are the line segments of the logo mark in favicon.svg, on its
// 32-unit grid, each drawn 2 units wide with round ends.
var logoStrokes = [][4]float64{
	{10, 6, 22, 6}, {22, 6, 22, 12}, {22, 12, 10, 12}, {10, 12, 10, 6}, // printer head
	{13, 12, 13, 16}, {13, 16, 19, 16}, {19, 16, 19, 12}, // hot end
	{14.5, 16, 16, 19.5}, {16, 19.5, 17.5, 16}, // nozzle
	{7, 25, 25, 25}, // bed
}

// renderIcon draws the logo in fg on a full-bleed square of bg, size pixels
// wide. The square has no rounded corners or transparency: iOS and Android
// launchers apply their own mask, and iOS fills transparency with black.
func renderIcon(size int, scale float64, bg, fg color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	unitsPerPx := 32 / (float64(size) * scale)
	for y := range size {
		for x := range size {
			// Pixel centre, in logo units.
			u := (float64(x)+0.5-float64(size)/2)*unitsPerPx + 16
			v := (float64(y)+0.5-float64(size)/2)*unitsPerPx + 16
			d := math.Inf(1)
			for _, s := range logoStrokes {
				d = min(d, segDist(u, v, s))
			}
			// Coverage from the distance to the stroke's edge, in pixels.
			a := math.Max(0, math.Min(1, (1-d)/unitsPerPx+0.5))
			img.SetRGBA(x, y, color.RGBA{mix(bg.R, fg.R, a), mix(bg.G, fg.G, a), mix(bg.B, fg.B, a), 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img) // writing to a bytes.Buffer cannot fail
	return buf.Bytes()
}

// segDist is the distance from (px, py) to the segment s.
func segDist(px, py float64, s [4]float64) float64 {
	dx, dy := s[2]-s[0], s[3]-s[1]
	t := ((px-s[0])*dx + (py-s[1])*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(px-(s[0]+t*dx), py-(s[1]+t*dy))
}

func mix(a, b uint8, t float64) uint8 {
	return uint8(math.Round(float64(a) + (float64(b)-float64(a))*t))
}

// parseHex reads a validated "#rrggbb".
func parseHex(hex string) color.RGBA {
	n, _ := strconv.ParseUint(hex[1:], 16, 32)
	return color.RGBA{uint8(n >> 16), uint8(n >> 8), uint8(n), 255} // #nosec G115 -- each byte is masked by the conversion
}
