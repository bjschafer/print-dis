package web

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/store"
)

// fakeIdP is a minimal OIDC provider: discovery, JWKS, authorize (by hand),
// and a token endpoint that checks PKCE and returns a signed ID token.
type fakeIdP struct {
	t   *testing.T
	srv *httptest.Server
	key *rsa.PrivateKey

	mu    sync.Mutex
	codes map[string]pendingCode // code -> what to issue
}

type pendingCode struct {
	challenge string
	nonce     string
	claims    map[string]any
}

func newFakeIdP(t *testing.T) *fakeIdP {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIdP{t: t, key: key, codes: map[string]pendingCode{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                f.srv.URL,
			"authorization_endpoint":                f.srv.URL + "/authorize",
			"token_endpoint":                        f.srv.URL + "/token",
			"jwks_uri":                              f.srv.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		pc, ok := f.codes[r.PostForm.Get("code")]
		delete(f.codes, r.PostForm.Get("code"))
		f.mu.Unlock()
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != pc.challenge {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		claims := map[string]any{
			"iss": f.srv.URL, "aud": "print-dis", "exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(), "nonce": pc.nonce,
		}
		for k, v := range pc.claims {
			claims[k] = v
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "token_type": "Bearer", "expires_in": 3600, "id_token": f.sign(claims),
		})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeIdP) sign(claims map[string]any) string {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: f.key}, (&jose.SignerOptions{}).WithHeader("kid", "k1"))
	if err != nil {
		f.t.Fatal(err)
	}
	payload, _ := json.Marshal(claims)
	jws, err := signer.Sign(payload)
	if err != nil {
		f.t.Fatal(err)
	}
	s, _ := jws.CompactSerialize()
	return s
}

func oidcHarness(t *testing.T) (*harness, *fakeIdP) {
	idp := newFakeIdP(t)
	h := newHarness(t, func(c *config.Config) {
		c.DevUser = ""
		c.OIDCIssuer = idp.srv.URL
		c.OIDCClientID = "print-dis"
		c.OIDCClientSecret = "secret"
		c.AdminGroup = "print-dis-admins"
	})
	return h, idp
}

// signIn runs the whole code flow as a fresh client with the given claims
// and returns that client and where the app sent it at the end.
func (h *harness) signIn(idp *fakeIdP, start string, claims map[string]any) (*harness, string) {
	h.t.Helper()
	c := h.as(nil)
	res, err := c.c.Get(h.srv.URL + start)
	if err != nil {
		h.t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/auth/login" {
		h.t.Fatalf("anonymous %s: %d %s", start, res.StatusCode, res.Header.Get("Location"))
	}
	res, err = c.c.Get(h.srv.URL + "/auth/login")
	if err != nil {
		h.t.Fatal(err)
	}
	_ = res.Body.Close()
	authz, err := url.Parse(res.Header.Get("Location"))
	if err != nil || !strings.HasPrefix(authz.String(), idp.srv.URL+"/authorize") {
		h.t.Fatalf("login redirect: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	q := authz.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || q.Get("nonce") == "" {
		h.t.Fatalf("authorize params missing PKCE or nonce: %v", q)
	}
	if !strings.Contains(q.Get("scope"), "openid") || !strings.Contains(q.Get("scope"), "email") {
		h.t.Fatalf("scopes: %q", q.Get("scope"))
	}
	code := "code-" + q.Get("state")[:8]
	idp.mu.Lock()
	idp.codes[code] = pendingCode{challenge: q.Get("code_challenge"), nonce: q.Get("nonce"), claims: claims}
	idp.mu.Unlock()
	res, err = c.c.Get(h.srv.URL + "/auth/callback?code=" + code + "&state=" + url.QueryEscape(q.Get("state")))
	if err != nil {
		h.t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		h.t.Fatalf("callback: %d", res.StatusCode)
	}
	return c, res.Header.Get("Location")
}

func TestOIDCSignIn(t *testing.T) {
	h, idp := oidcHarness(t)
	ctx := context.Background()

	// The owner is an admin by group, and lands where they started.
	owner, to := h.signIn(idp, "/printers", map[string]any{
		"sub": "owner-sub", "email": "Owner@Example.com", "email_verified": true, "name": "The Owner",
		"groups": []string{"print-dis-admins", "other"},
	})
	if to != "/printers" {
		t.Fatalf("return to = %q", to)
	}
	if code, body := owner.get("/printers"); code != http.StatusOK {
		t.Fatalf("owner printers: %d %s", code, body)
	}

	// A placeholder filed by the owner, with a request.
	if code, _, _ := owner.post("/requests", url.Values{"title": {"For Carol"}, "new_person_name": {"Carol"}, "new_person_email": {"carol@example.com"}}); code != http.StatusSeeOther {
		t.Fatalf("file for placeholder: %d", code)
	}
	carolPH, err := h.store.GetUserByEmail(ctx, "carol@example.com")
	if err != nil || !carolPH.Placeholder() {
		t.Fatalf("placeholder: %+v %v", carolPH, err)
	}

	// Carol signs in with an unverified email: she must not inherit it.
	unverified, _ := h.signIn(idp, "/", map[string]any{"sub": "carol-sub", "email": "carol@example.com", "email_verified": false, "name": "Carol"})
	_, body := unverified.get("/")
	unverified.mustNotContain(body, "For Carol")

	// The same person with a verified email under a second identity links the placeholder.
	carol, _ := h.signIn(idp, "/", map[string]any{"sub": "carol-sub-2", "email": "carol@example.com", "email_verified": true, "name": "Carol R"})
	_, body = carol.get("/")
	carol.mustContain(body, "For Carol")
	carol.mustNotContain(body, `href="/printers"`)
	linked, _ := h.store.GetUserByEmail(ctx, "carol@example.com")
	if linked.ID != carolPH.ID || linked.Placeholder() || linked.Name != "Carol R" || linked.IsAdmin {
		t.Fatalf("linked user: %+v", linked)
	}
	if code, _ := carol.get("/printers"); code != http.StatusForbidden {
		t.Fatalf("non-admin printers: %d", code)
	}

	// Admin is refreshed from groups at each sign-in.
	again, _ := h.signIn(idp, "/", map[string]any{"sub": "owner-sub", "email": "owner@example.com", "email_verified": true, "name": "The Owner", "groups": []string{}})
	if code, _ := again.get("/printers"); code != http.StatusForbidden {
		t.Fatalf("demoted owner printers: %d", code)
	}

	// Logout ends the session.
	if code, loc, _ := carol.post("/auth/logout", nil); code != http.StatusSeeOther || loc != "/auth/signed-out" {
		t.Fatalf("logout: %d %s", code, loc)
	}
	res, _ := carol.c.Get(h.srv.URL + "/")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/auth/login" {
		t.Fatalf("after logout: %d", res.StatusCode)
	}
}

func TestOIDCCallbackRejectsBadState(t *testing.T) {
	h, _ := oidcHarness(t)
	c := h.as(nil)
	res, err := c.c.Get(h.srv.URL + "/auth/login")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	res, err = c.c.Get(h.srv.URL + "/auth/callback?code=x&state=forged")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("forged state: %d", res.StatusCode)
	}
}

func TestSignInOIDCRules(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	s := h.store

	ph, _ := s.CreatePlaceholder(ctx, "Dana", "dana@example.com")
	// Verified email links the placeholder.
	u, err := s.SignInOIDC(ctx, store.OIDCIdentity{Subject: "d1", Email: "DANA@example.com", EmailVerified: true, Name: "Dana"})
	if err != nil || u.ID != ph.ID {
		t.Fatalf("link: %+v %v", u, err)
	}
	// A second identity with the same verified email gets a new row without the email.
	u2, err := s.SignInOIDC(ctx, store.OIDCIdentity{Subject: "d2", Email: "dana@example.com", EmailVerified: true, Name: "Imposter"})
	if err != nil || u2.ID == ph.ID || u2.Email != nil {
		t.Fatalf("second identity: %+v %v", u2, err)
	}
	// Merging two signed-in people is refused; merging a placeholder in works.
	if err := s.MergeUsers(ctx, u2.ID, u.ID); err == nil {
		t.Fatal("merged two signed-in users")
	}
	ph2, _ := s.CreatePlaceholder(ctx, "Dana (old)", "")
	r := &core.Request{RequesterID: ph2.ID, CreatedByID: ph2.ID, Title: "old", Quantity: 1, Status: core.StatusRequested, Timing: core.TimingWhenever}
	if err := s.CreateRequest(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeUsers(ctx, ph2.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetRequest(ctx, r.ID)
	if got.RequesterID != u.ID {
		t.Fatalf("request not moved: %+v", got)
	}
	if _, err := s.GetUser(ctx, ph2.ID); err == nil {
		t.Fatal("merged placeholder still exists")
	}
	// Merging a signed-in person into a placeholder hands over the identity.
	ph3, _ := s.CreatePlaceholder(ctx, "Erin", "erin@example.com")
	erin, _ := s.SignInOIDC(ctx, store.OIDCIdentity{Subject: "e1", Email: "erin2@example.com", EmailVerified: true, Name: "Erin"})
	if err := s.MergeUsers(ctx, erin.ID, ph3.ID); err != nil {
		t.Fatal(err)
	}
	again, err := s.SignInOIDC(ctx, store.OIDCIdentity{Subject: "e1", Name: "Erin"})
	if err != nil || again.ID != ph3.ID || *again.Email != "erin@example.com" {
		t.Fatalf("identity not handed over: %+v %v", again, err)
	}
}

func TestPeoplePage(t *testing.T) {
	h := newHarness(t)
	if code, _, _ := h.post("/people", url.Values{"name": {"Frank"}, "email": {"frank@example.com"}}); code != http.StatusSeeOther {
		t.Fatalf("add person: %d", code)
	}
	if code, _, body := h.post("/people", url.Values{"name": {"Frank 2"}, "email": {"FRANK@example.com"}}); code != http.StatusUnprocessableEntity || !strings.Contains(body, "already exists") {
		t.Fatalf("duplicate person: %d", code)
	}
	_, body := h.get("/people")
	h.mustContain(body, "Frank", "not signed in yet")
	frank, _ := h.store.GetUserByEmail(context.Background(), "frank@example.com")
	owner, _ := h.store.GetUserByEmail(context.Background(), "owner@example.com")
	if code, _, body := h.post("/people/merge", url.Values{"from": {itoa(owner.ID)}, "into": {itoa(frank.ID)}}); code != http.StatusUnprocessableEntity || !strings.Contains(body, "merge yourself away") {
		t.Fatalf("merge self away: %d", code)
	}
	friend := h.friend("Gina")
	if code, _ := h.as(friend).get("/people"); code != http.StatusForbidden {
		t.Fatalf("people as requester: %d", code)
	}
}
