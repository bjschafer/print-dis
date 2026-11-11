package web

import (
	"bytes"
	"context"
	"errors"
	"html"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/preview"
)

type fakePreviewer struct {
	p     *preview.Preview
	err   error
	calls atomic.Int32
	last  atomic.Value
}

func (f *fakePreviewer) Fetch(_ context.Context, raw string) (*preview.Preview, error) {
	f.calls.Add(1)
	f.last.Store(raw)
	return f.p, f.err
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestNewRequestPrefillsFromLink(t *testing.T) {
	h := newHarness(t)
	img := testPNG(t)
	fp := &fakePreviewer{p: &preview.Preview{Title: "Cable <clip>", Image: img, ImageType: "image/png"}}
	h.s.Previewer = fp

	link := "https://www.printables.com/model/42"
	code, body := h.get("/requests/new?url=" + url.QueryEscape(link))
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	h.mustContain(body, `value="Cable &lt;clip&gt;"`, link, `src="data:image/png;base64,`, `name="fetch_image"`, `name="source_title"`)
	h.mustNotContain(body, "read that page")

	// Create: the server fetches the thumbnail itself; a forged image field is ignored.
	code, loc, _ := h.post("/requests", url.Values{
		"title": {"Cable clip"}, "source_url": {link}, "source_title": {"Cable <clip>"}, "fetch_image": {"1"},
		"source_image": {"AAAA"}, "source_image_type": {"text/html"},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("create: %d", code)
	}
	if fp.calls.Load() != 2 {
		t.Errorf("previewer calls = %d, want 2 (form, create)", fp.calls.Load())
	}
	res, err := h.c.Get(h.srv.URL + loc + "/image")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	got, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/png" || !bytes.Equal(got, img) {
		t.Errorf("image: %d %q %d bytes", res.StatusCode, res.Header.Get("Content-Type"), len(got))
	}
	_, detail := h.get(loc)
	h.mustContain(detail, loc+"/image")
}

func TestNewRequestWithoutFetchImageDoesNotFetch(t *testing.T) {
	h := newHarness(t)
	fp := &fakePreviewer{p: &preview.Preview{Title: "T", Image: testPNG(t), ImageType: "image/png"}}
	h.s.Previewer = fp
	code, loc, _ := h.post("/requests", url.Values{"title": {"T"}, "source_url": {"https://example.com/m"}})
	if code != http.StatusSeeOther {
		t.Fatalf("create: %d", code)
	}
	if fp.calls.Load() != 0 {
		t.Error("fetched without the flag")
	}
	if c, _ := h.get(loc + "/image"); c != http.StatusNotFound {
		t.Errorf("image status %d", c)
	}
}

func TestNewRequestPreviewFailureStillUsable(t *testing.T) {
	h := newHarness(t)
	fp := &fakePreviewer{err: errors.New("403 from model site")}
	h.s.Previewer = fp
	link := "https://makerworld.com/en/models/1"
	code, body := h.get("/requests/new?url=" + url.QueryEscape(link))
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	h.mustContain(body, link, "Couldn&#39;t read that page; add a title yourself.", `name="title"`)
	h.mustNotContain(body, "fetch_image", "data:image", "403")

	// Filing it works, with no image and no re-fetch.
	code, loc, _ := h.post("/requests", url.Values{"title": {"Mine"}, "source_url": {link}})
	if code != http.StatusSeeOther {
		t.Fatalf("create: %d", code)
	}
	if c, _ := h.get(loc + "/image"); c != http.StatusNotFound {
		t.Errorf("image status %d", c)
	}
}

func TestNewRequestIgnoresBadLinks(t *testing.T) {
	h := newHarness(t)
	fp := &fakePreviewer{p: &preview.Preview{Title: "x"}}
	h.s.Previewer = fp
	for _, raw := range []string{"javascript:alert(1)", "file:///etc/passwd", "not a url", ""} {
		if code, _ := h.get("/requests/new?url=" + url.QueryEscape(raw)); code != http.StatusOK {
			t.Errorf("%q: status %d", raw, code)
		}
	}
	if fp.calls.Load() != 0 {
		t.Error("previewer called for a non-http URL")
	}
}

func TestNewRequestWithoutPreviewer(t *testing.T) {
	h := newHarness(t)
	code, body := h.get("/requests/new?url=" + url.QueryEscape("https://example.com/a"))
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	h.mustContain(body, "https://example.com/a")
}

func TestHelpBookmarklet(t *testing.T) {
	h := newHarness(t, func(c *config.Config) {
		c.BaseURL, _ = url.Parse("https://prints.example.org/")
	})
	code, body := h.get("/help")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	m := regexp.MustCompile(`href="(javascript:[^"]+)"`).FindStringSubmatch(body)
	if m == nil {
		t.Fatal("no javascript: bookmarklet link")
	}
	// The browser decodes entities and percent-escapes before running it.
	js, err := url.PathUnescape(html.UnescapeString(m[1]))
	if err != nil {
		t.Fatal(err)
	}
	h.mustContain(js, "window.open('https://prints.example.org/requests/new?url='+encodeURIComponent(location.href)")
	h.mustContain(body, "phone", `href="/help"`)
	h.mustNotContain(body, "ZgotmplZ", "prints.example.org//")
}

func TestBookmarkletEscapesBase(t *testing.T) {
	got := string(bookmarklet(`https://x.example/a'b\c`))
	if !strings.Contains(got, `'https://x.example/a\'b\\c/requests/new?url='`) {
		t.Errorf("not escaped: %s", got)
	}
}

// The real fetcher built the way main builds it refuses loopback, so the
// new-request form cannot be used to probe the local network.
func TestRealFetcherRefusesLoopback(t *testing.T) {
	h := newHarness(t)
	h.s.Previewer = preview.New()
	code, body := h.get("/requests/new?url=" + url.QueryEscape("http://127.0.0.1:1/"))
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	h.mustContain(body, "Couldn&#39;t read that page")
}
