package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/bjschafer/print-dis/internal/auth"
	"github.com/bjschafer/print-dis/internal/core"
)

// owner returns the DEV_USER admin, signing the harness client in as them.
func (h *harness) owner() *core.User {
	h.t.Helper()
	h.get("/")
	u, err := h.store.GetUserByEmail(context.Background(), "owner@example.com")
	if err != nil {
		h.t.Fatal(err)
	}
	return u
}

func (h *harness) token(u *core.User) string {
	h.t.Helper()
	tok, hash := auth.NewAPIToken()
	if err := h.store.CreateAPIToken(context.Background(), u.ID, "test", hash); err != nil {
		h.t.Fatal(err)
	}
	return tok
}

// api calls the API with no cookies. body is sent as is when it is []byte,
// and as JSON otherwise. The reply is decoded into out when out is not nil.
func (h *harness) api(method, path, token string, body, out any) (int, http.Header, string) {
	h.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		rd = bytes.NewReader(b)
	case string:
		rd = strings.NewReader(b)
	default:
		j, err := json.Marshal(b)
		if err != nil {
			h.t.Fatal(err)
		}
		rd = bytes.NewReader(j)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	if out != nil && res.StatusCode < 300 {
		if err := json.Unmarshal(raw, out); err != nil {
			h.t.Fatalf("%s %s: decoding %q: %v", method, path, raw, err)
		}
	}
	return res.StatusCode, res.Header, string(raw)
}

func TestAPIAuth(t *testing.T) {
	h := newHarness(t)
	admin := h.owner()
	tok := h.token(admin)
	friendTok := h.token(h.friend("Alice"))

	code, hdr, _ := h.api("GET", "/api/v1/requests", "", nil, nil)
	if code != http.StatusUnauthorized || hdr.Get("WWW-Authenticate") == "" {
		t.Errorf("no token: %d %q", code, hdr.Get("WWW-Authenticate"))
	}
	if code, _, _ := h.api("GET", "/api/v1/requests", "pd_nope", nil, nil); code != http.StatusUnauthorized {
		t.Errorf("unknown token: %d", code)
	}
	if code, _, _ := h.api("GET", "/api/v1/requests", friendTok, nil, nil); code != http.StatusForbidden {
		t.Errorf("non-admin token: %d", code)
	}
	if code, _, body := h.api("GET", "/api/v1/requests", tok, nil, nil); code != http.StatusOK || strings.TrimSpace(body) != "[]" {
		t.Errorf("admin token: %d %s", code, body)
	}

	// A signed-in admin's session cookie does not open the API.
	res, err := h.c.Get(h.srv.URL + "/api/v1/requests")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("session cookie: %d", res.StatusCode)
	}

	toks, err := h.store.ListAPITokens(context.Background(), admin.ID)
	if err != nil || len(toks) != 1 || toks[0].LastUsedAt == nil {
		t.Fatalf("tokens %+v, %v", toks, err)
	}
	if err := h.store.DeleteAPIToken(context.Background(), admin.ID, toks[0].ID); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := h.api("GET", "/api/v1/requests", tok, nil, nil); code != http.StatusUnauthorized {
		t.Errorf("revoked token: %d", code)
	}
}

func TestAPICreateIsIdempotentByKey(t *testing.T) {
	h := newHarness(t)
	tok := h.token(h.owner())

	body := map[string]any{
		"key": "voron-trident:STLs/Gantry/idler.stl", "project": "Voron Trident", "title": "idler",
		"quantity": 2, "material": "abs", "colors": []map[string]any{{"rough": "red", "note": "accent"}},
	}
	var got apiRequestJSON
	code, hdr, raw := h.api("POST", "/api/v1/requests", tok, body, &got)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %s", code, raw)
	}
	if hdr.Get("Location") != "/api/v1/requests/"+itoa(got.ID) || got.Status != core.StatusRequested ||
		got.Quantity != 2 || got.Material != "ABS" || !got.NeedsEnclosure || got.NeedsMultiMaterial ||
		got.Project != "Voron Trident" || got.Key == nil || *got.Key != body["key"] ||
		len(got.Colors) != 1 || got.Colors[0].Rough != "red" || got.Colors[0].Note != "accent" ||
		got.URL != h.s.Config.BaseURL.String()+"/requests/"+itoa(got.ID) {
		t.Errorf("created %+v (Location %q)", got, hdr.Get("Location"))
	}

	var again apiRequestJSON
	if code, _, raw := h.api("POST", "/api/v1/requests", tok, body, &again); code != http.StatusOK || again.ID != got.ID {
		t.Errorf("re-run: %d %s", code, raw)
	}
	var list []apiRequestJSON
	h.api("GET", "/api/v1/requests?project="+url.QueryEscape("Voron Trident"), tok, nil, &list)
	if len(list) != 1 {
		t.Errorf("project list: %d", len(list))
	}
	h.api("GET", "/api/v1/requests?key="+url.QueryEscape("voron-trident:STLs/Gantry/idler.stl"), tok, nil, &list)
	if len(list) != 1 || list[0].ID != got.ID {
		t.Errorf("key list: %+v", list)
	}

	// An explicit false overrides the material's enclosure default.
	var pla apiRequestJSON
	h.api("POST", "/api/v1/requests", tok, map[string]any{"title": "clip", "material": "ASA", "needs_enclosure": false}, &pla)
	if pla.NeedsEnclosure || pla.Quantity != 1 || pla.Key != nil {
		t.Errorf("explicit false: %+v", pla)
	}

	for _, tt := range []struct {
		body string
		code int
		want string
	}{
		{`{"titel": "x"}`, http.StatusBadRequest, `unknown field \"titel\"`},
		{`{"title": ""}`, http.StatusUnprocessableEntity, "Give the request a title."},
		{`{"title": "x", "colors": [{"rough": "red", "hex": "#ff0000"}]}`, http.StatusUnprocessableEntity, "only one of"},
		{`{"title": "x", "colors": [{}]}`, http.StatusUnprocessableEntity, "Color 1"},
		{`{"title": "x", "colors": [{"rough": "mauve"}]}`, http.StatusUnprocessableEntity, "Pick a color"},
		{`{"title": "x", "project": "!nope"}`, http.StatusUnprocessableEntity, "project name"},
		{`{"title": "x", "requester_id": 99999}`, http.StatusUnprocessableEntity, "requester_id 99999"},
	} {
		code, _, raw := h.api("POST", "/api/v1/requests", tok, tt.body, nil)
		if code != tt.code || !strings.Contains(raw, tt.want) {
			t.Errorf("%s: %d %s", tt.body, code, raw)
		}
	}
}

func TestAPIUploadStatusAndPrinted(t *testing.T) {
	h := newHarness(t, withFiles(t.TempDir(), 200<<20))
	tok := h.token(h.owner())
	var req apiRequestJSON
	h.api("POST", "/api/v1/requests", tok, map[string]any{"title": "idler", "quantity": 2}, &req)
	base := "/api/v1/requests/" + itoa(req.ID)

	var f apiFileJSON
	if code, _, raw := h.api("POST", base+"/files?name=idler.stl", tok, cubeSTL(10, 20, 5), &f); code != http.StatusCreated {
		t.Fatalf("upload: %d %s", code, raw)
	}
	if f.Filename != "idler.stl" || f.Size == nil || *f.Size != (apiDims{10, 20, 5}) {
		t.Errorf("file %+v", f)
	}
	h.api("GET", base, tok, nil, &req)
	if req.Size == nil || *req.Size != (apiDims{20, 10, 5}) {
		t.Errorf("request size from upload: %+v", req.Size)
	}
	var fs []apiFileJSON
	h.api("GET", base+"/files", tok, nil, &fs)
	if len(fs) != 1 || fs[0].SHA256 != f.SHA256 {
		t.Errorf("files %+v", fs)
	}
	if code, _, _ := h.api("POST", base+"/files?name=run.exe", tok, []byte("x"), nil); code != http.StatusUnsupportedMediaType {
		t.Errorf("bad type: %d", code)
	}
	if code, _, _ := h.api("POST", base+"/files", tok, []byte("x"), nil); code != http.StatusBadRequest {
		t.Errorf("no name: %d", code)
	}

	for _, st := range []string{"printing", "printing"} {
		if code, _, raw := h.api("POST", base+"/status", tok, map[string]string{"status": st}, &req); code != http.StatusOK || req.Status != core.StatusPrinting {
			t.Errorf("status %s: %d %s", st, code, raw)
		}
	}
	if code, _, _ := h.api("POST", base+"/status", tok, map[string]string{"status": "done"}, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("bad status: %d", code)
	}
	if code, _, raw := h.api("POST", base+"/printed", tok, map[string]int{"printed": 1}, &req); code != http.StatusOK || req.Printed != 1 {
		t.Errorf("printed: %d %s", code, raw)
	}
	if code, _, _ := h.api("POST", base+"/printed", tok, map[string]int{"printed": -1}, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("negative printed: %d", code)
	}
	if code, _, _ := h.api("GET", "/api/v1/requests/999999", tok, nil, nil); code != http.StatusNotFound {
		t.Errorf("missing request: %d", code)
	}

	evs, err := h.store.ListEvents(context.Background(), req.ID)
	if err != nil {
		t.Fatal(err)
	}
	var bodies []string
	for _, e := range evs {
		bodies = append(bodies, e.Body)
	}
	got := strings.Join(bodies, "|")
	for _, want := range []string{"uploaded idler.stl", "requested -> printing", "changed printed"} {
		if !strings.Contains(got, want) {
			t.Errorf("events %q missing %q", got, want)
		}
	}
}

var tokenRe = regexp.MustCompile(`value="(pd_[A-Za-z0-9_-]+)"`)

func TestTokenShownOnceAndWorks(t *testing.T) {
	h := newHarness(t)
	h.owner()
	code, loc, _ := h.post("/tokens", url.Values{"name": {"voron script"}})
	if code != http.StatusSeeOther || loc != "/help#api" {
		t.Fatalf("create token: %d %q", code, loc)
	}
	_, body := h.get("/help")
	m := tokenRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("new token not shown")
	}
	h.mustContain(body, "voron script", "never used")
	if _, body := h.get("/help"); strings.Contains(body, m[1]) {
		t.Error("token shown a second time")
	}
	if code, _, _ := h.api("GET", "/api/v1/requests", m[1], nil, nil); code != http.StatusOK {
		t.Errorf("token from the page: %d", code)
	}

	alice := h.as(h.friend("Alice"))
	if code, _, _ := alice.post("/tokens", url.Values{"name": {"x"}}); code != http.StatusForbidden {
		t.Errorf("non-admin create: %d", code)
	}
	_, body = alice.get("/help")
	alice.mustNotContain(body, `id="api"`)
}

func TestProjectFilterOnBoard(t *testing.T) {
	h := newHarness(t)
	tok := h.token(h.owner())
	h.api("POST", "/api/v1/requests", tok, map[string]any{"title": "Trident idler", "project": "Voron Trident"}, nil)
	h.api("POST", "/api/v1/requests", tok, map[string]any{"title": "Headphone stand"}, nil)

	for _, tt := range []struct {
		project   string
		want, not []string
	}{
		{"", []string{"Trident idler", "Headphone stand"}, nil},
		{"Voron Trident", []string{"Trident idler"}, []string{"Headphone stand"}},
		{"!Voron Trident", []string{"Headphone stand"}, []string{"Trident idler"}},
		{"-", []string{"Headphone stand"}, []string{"Trident idler"}},
	} {
		_, body := h.get("/?project=" + url.QueryEscape(tt.project))
		h.mustContain(body, tt.want...)
		h.mustNotContain(body, tt.not...)
	}
	_, body := h.get("/requests?sort=project")
	h.mustContain(body, "<td>Voron Trident</td>")
}
