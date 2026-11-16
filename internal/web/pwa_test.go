package web

import (
	"encoding/json"
	"image/color"
	"image/png"
	"net/http"
	"strings"
	"testing"

	"github.com/bjschafer/print-dis/internal/config"
)

func TestManifestAndIcons(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.AccentColor = "#1e3a8a" })
	_, page := h.get("/")
	h.mustContain(page, `<link rel="manifest" href="/manifest.webmanifest"`, `<link rel="apple-touch-icon" href="/apple-touch-icon.png">`)

	// Fetched without a session, as browsers fetch manifests and icons.
	anon := h.as(nil)
	code, body := anon.get("/manifest.webmanifest")
	if code != http.StatusOK {
		t.Fatalf("manifest: %d", code)
	}
	var m struct {
		StartURL string `json:"start_url"`
		Display  string `json:"display"`
		Icons    []struct{ Src, Sizes, Purpose string }
	}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("manifest: %v\n%s", err, body)
	}
	if m.StartURL != "/" || m.Display != "standalone" || len(m.Icons) == 0 {
		t.Fatalf("manifest: %+v", m)
	}

	want := map[string]string{"/apple-touch-icon.png": "180x180"}
	for _, ic := range m.Icons {
		want[ic.Src] = ic.Sizes
	}
	for src, sizes := range want {
		res, err := anon.c.Get(h.srv.URL + src)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(res.Body)
		_ = res.Body.Close()
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		b := img.Bounds()
		if got := strings.Join([]string{itoa(int64(b.Dx())), itoa(int64(b.Dy()))}, "x"); got != sizes {
			t.Errorf("%s is %s, manifest says %s", src, got, sizes)
		}
		// Opaque accent at the corner; the logo's bed line in the text color.
		if got := color.RGBAModel.Convert(img.At(0, 0)); got != (color.RGBA{0x1e, 0x3a, 0x8a, 255}) {
			t.Errorf("%s corner = %v", src, got)
		}
		if got := color.RGBAModel.Convert(img.At(b.Dx()/2, b.Dy()*25/32)); got != (color.RGBA{255, 255, 255, 255}) && !strings.Contains(src, "maskable") {
			t.Errorf("%s bed line = %v", src, got)
		}
	}
}
