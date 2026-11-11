package web

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/files"
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
	return newHarnessDeps(t, nil, mutate...)
}

// newHarnessDeps is newHarness with a hook to adjust Deps before the server is built.
func newHarnessDeps(t *testing.T, deps func(*Deps), mutate ...func(*config.Config)) *harness {
	t.Helper()
	st := storetest.New(t)
	u, _ := url.Parse("http://localhost:8080")
	cfg := &config.Config{BaseURL: u, DevUser: "owner@example.com", UploadMaxBytes: 200 << 20}
	for _, m := range mutate {
		m(cfg)
	}
	// Tests that set FilesBackend get a real backend; the default is none.
	blobs, err := files.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	d := Deps{Config: cfg, Store: st, Files: blobs}
	if deps != nil {
		deps(&d)
	}
	s, err := New(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	// /test/login/{id} signs the client in as any user, standing in for OIDC.
	mux := http.NewServeMux()
	mux.Handle("/", s)
	mux.Handle("POST /test/login/{id}", s.Auth.Sessions.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		u, err := st.GetUser(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if err := s.Auth.SignIn(r.Context(), u); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))
	srv := httptest.NewServer(mux)
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

// as returns a new harness client, sharing the server, signed in as u
// (or anonymous when u is nil).
func (h *harness) as(u *core.User) *harness {
	h.t.Helper()
	jar, _ := cookiejar.New(nil)
	other := *h
	other.csrf = ""
	other.c = &http.Client{Jar: jar, CheckRedirect: h.c.CheckRedirect}
	if u == nil { // anonymous
		return &other
	}
	res, err := other.c.Post(h.srv.URL+"/test/login/"+strconv.FormatInt(u.ID, 10), "", nil)
	if err != nil {
		h.t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		h.t.Fatalf("test login as %d: %d", u.ID, res.StatusCode)
	}
	return &other
}

// friend creates a non-admin user who has signed in before.
func (h *harness) friend(name string) *core.User {
	h.t.Helper()
	var u core.User
	err := h.store.Pool.QueryRow(context.Background(),
		`insert into users (oidc_sub, email, name) values ($1, $2, $3) returning id, name`,
		"sub-"+name, strings.ToLower(name)+"@example.com", name).Scan(&u.ID, &u.Name)
	if err != nil {
		h.t.Fatal(err)
	}
	return &u
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
