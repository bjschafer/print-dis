package web

import (
	"testing"

	"github.com/bjschafer/print-dis/internal/config"
)

func TestTextOn(t *testing.T) {
	for hex, want := range map[string]string{
		"#00ae42": "#0b0d10", // default green: dark text
		"#ffffff": "#0b0d10",
		"#ffd400": "#0b0d10",
		"#1e3a8a": "#ffffff", // navy: white text
		"#7c3aed": "#ffffff",
		"#000000": "#ffffff",
	} {
		if got := textOn(hex); got != want {
			t.Errorf("textOn(%s) = %s, want %s", hex, got, want)
		}
	}
	if got := string(accentCSS("#1e3a8a")); got != "--brand: #1e3a8a; --brand-text: #ffffff;" {
		t.Errorf("accentCSS = %q", got)
	}
}

func TestAccentColorPages(t *testing.T) {
	h := newHarness(t)
	_, body := h.get("/")
	h.mustNotContain(body, "--brand:")
	_, svg := h.get("/favicon.svg")
	h.mustContain(svg, `fill="#00ae42"`)

	h = newHarness(t, func(c *config.Config) { c.AccentColor = "#1e3a8a" })
	_, body = h.get("/")
	h.mustContain(body, "<style>:root { --brand: #1e3a8a; --brand-text: #ffffff; }</style>")
	_, svg = h.get("/favicon.svg")
	h.mustContain(svg, `fill="#1e3a8a"`, `stroke="#ffffff"`)
}
