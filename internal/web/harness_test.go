package web

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/store"
	"github.com/bjschafer/print-dis/internal/store/storetest"
)

// harness is a running server plus a cookie-carrying client.
type harness struct {
	t     *testing.T
	srv   *httptest.Server
	s     *Server
	store *store.Store
	c     *http.Client
	csrf  string
}

func newHarness(t *testing.T, mutate ...func(*config.Config)) *harness {
	t.Helper()
	st := storetest.New(t)
	u, _ := url.Parse("http://localhost:8080")
	cfg := &config.Config{BaseURL: u, DevUser: "owner@example.com", UploadMaxBytes: 200 << 20}
	for _, m := range mutate {
		m(cfg)
	}
	s, err := New(context.Background(), Deps{Config: cfg, Store: st})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	h := &harness{t: t, srv: srv, s: s, store: st, c: &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
	return h
}

var csrfRe = regexp.MustCompile(`data-csrf="([^"]+)"`)

// get fetches path and returns status and body, remembering the CSRF token.
func (h *harness) get(path string) (int, string) {
	h.t.Helper()
	res, err := h.c.Get(h.srv.URL + path)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	if m := csrfRe.FindSubmatch(b); m != nil {
		h.csrf = string(m[1])
	}
	return res.StatusCode, string(b)
}

// post sends a form with the CSRF token and returns status, Location and body.
func (h *harness) post(path string, form url.Values, headers ...string) (int, string, string) {
	h.t.Helper()
	if h.csrf == "" {
		h.get("/")
	}
	if form == nil {
		form = url.Values{}
	}
	if form.Get("csrf") == "" {
		form.Set("csrf", h.csrf)
	}
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := h.c.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header.Get("Location"), string(b)
}

func (h *harness) mustContain(body string, want ...string) {
	h.t.Helper()
	for _, w := range want {
		if !strings.Contains(body, w) {
			h.t.Errorf("body missing %q", w)
		}
	}
}

func (h *harness) mustNotContain(body string, bad ...string) {
	h.t.Helper()
	for _, w := range bad {
		if strings.Contains(body, w) {
			h.t.Errorf("body unexpectedly contains %q", w)
		}
	}
}
