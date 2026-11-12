package web

import (
	"fmt"
	"html/template"
	"math"
	"net/http"
	"strconv"
)

const defaultAccent = "#00ae42"

// accent is the configured accent color, or the default.
func (s *Server) accent() string {
	if s.Config != nil && s.Config.AccentColor != "" {
		return s.Config.AccentColor
	}
	return defaultAccent
}

// themeCSS is the custom-property override for a configured accent, or ""
// when the default is in use and app.css already has it.
func (s *Server) themeCSS() template.CSS {
	if s.Config == nil || s.Config.AccentColor == "" {
		return ""
	}
	return accentCSS(s.Config.AccentColor)
}

// accentCSS sets --brand to hex and --brand-text to whichever of near-black
// or white reads better on it. hex must already be a valid "#rrggbb".
func accentCSS(hex string) template.CSS {
	return template.CSS(fmt.Sprintf("--brand: %s; --brand-text: %s;", hex, textOn(hex))) // #nosec G203 -- validated hex
}

// textOn picks near-black or white, whichever contrasts more with hex.
func textOn(hex string) string {
	if l := luminance(hex); contrast(l, luminance("#0b0d10")) > contrast(l, 1) {
		return "#0b0d10"
	}
	return "#ffffff"
}

// luminance is the WCAG relative luminance of "#rrggbb".
func luminance(hex string) float64 {
	n, _ := strconv.ParseUint(hex[1:], 16, 32)
	ch := func(v uint64) float64 {
		c := float64(v) / 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(n>>16&0xff) + 0.7152*ch(n>>8&0xff) + 0.0722*ch(n&0xff)
}

func contrast(a, b float64) float64 { return (max(a, b) + 0.05) / (min(a, b) + 0.05) }

// favicon is the logo mark in the accent color.
func (s *Server) favicon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="%s"/><path d="M10 6h12v6H10zM13 12v4h6v-4M14.5 16 16 19.5 17.5 16M7 25h18" fill="none" stroke="%s" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`, s.accent(), textOn(s.accent()))
}
