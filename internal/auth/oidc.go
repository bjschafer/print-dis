package auth

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/store"
)

const (
	keyReturnTo = "return_to"
	keyState    = "oidc_state"
	keyNonce    = "oidc_nonce"
	keyVerifier = "oidc_verifier"
)

// oidcClient discovers the issuer lazily, so the app starts while the
// identity provider is down and recovers when it comes back.
type oidcClient struct {
	cfg *config.Config

	mu       sync.Mutex
	provider *oidc.Provider
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
}

func newOIDCClient(_ context.Context, cfg *config.Config) (*oidcClient, error) {
	return &oidcClient{cfg: cfg}, nil
}

func (c *oidcClient) discover(ctx context.Context) (*oauth2.Config, *oidc.IDTokenVerifier, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.provider != nil {
		return c.oauth, c.verifier, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	p, err := oidc.NewProvider(ctx, c.cfg.OIDCIssuer)
	if err != nil {
		return nil, nil, fmt.Errorf("discover %s: %w", c.cfg.OIDCIssuer, err)
	}
	c.provider = p
	c.oauth = &oauth2.Config{
		ClientID:     c.cfg.OIDCClientID,
		ClientSecret: c.cfg.OIDCClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  c.cfg.BaseURL.String() + "/auth/callback",
		// "groups" is served by a custom scope mapping in tf/authentik; the
		// built-in profile scope may also include it.
		Scopes: []string{oidc.ScopeOpenID, "email", "profile", "groups"},
	}
	c.verifier = p.Verifier(&oidc.Config{ClientID: c.cfg.OIDCClientID})
	return c.oauth, c.verifier, nil
}

// Claims are the ID token claims print-dis uses.
type Claims struct {
	Subject           string   `json:"sub"`
	Email             string   `json:"email"`
	EmailVerified     *bool    `json:"email_verified"`
	Name              string   `json:"name"`
	PreferredUsername string   `json:"preferred_username"`
	Groups            []string `json:"groups"`
}

// Login starts the authorization code flow with PKCE.
func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	if a.oidc == nil { // DEV_USER: already signed in by the middleware
		http.Redirect(w, r, a.popReturnTo(r.Context()), http.StatusSeeOther)
		return
	}
	oc, _, err := a.oidc.discover(r.Context())
	if err != nil {
		a.log.Error("oidc discovery", "err", err)
		http.Error(w, "Sign-in is unavailable right now. Try again in a minute.", http.StatusServiceUnavailable)
		return
	}
	state, nonce, verifier := randomToken(), randomToken(), oauth2.GenerateVerifier()
	ctx := r.Context()
	a.Sessions.Put(ctx, keyState, state)
	a.Sessions.Put(ctx, keyNonce, nonce)
	a.Sessions.Put(ctx, keyVerifier, verifier)
	http.Redirect(w, r, oc.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

// Callback finishes the flow, creates or links the user, and signs them in.
func (a *Auth) Callback(w http.ResponseWriter, r *http.Request) {
	if a.oidc == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	ctx := r.Context()
	state := a.Sessions.PopString(ctx, keyState)
	nonce := a.Sessions.PopString(ctx, keyNonce)
	verifier := a.Sessions.PopString(ctx, keyVerifier)
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		a.log.Warn("oidc error from provider", "error", e, "description", q.Get("error_description"))
		http.Error(w, "Sign-in was cancelled or refused.", http.StatusForbidden)
		return
	}
	if state == "" || q.Get("state") != state {
		http.Error(w, "That sign-in link has expired. Go back and try again.", http.StatusBadRequest)
		return
	}
	oc, ver, err := a.oidc.discover(ctx)
	if err != nil {
		a.log.Error("oidc discovery", "err", err)
		http.Error(w, "Sign-in is unavailable right now. Try again in a minute.", http.StatusServiceUnavailable)
		return
	}
	tok, err := oc.Exchange(ctx, q.Get("code"), oauth2.VerifierOption(verifier))
	if err != nil {
		a.log.Warn("oidc exchange", "err", err)
		http.Error(w, "Sign-in failed. Go back and try again.", http.StatusBadRequest)
		return
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok {
		http.Error(w, "Sign-in failed: no identity token.", http.StatusBadRequest)
		return
	}
	idt, err := ver.Verify(ctx, raw)
	if err != nil || idt.Nonce != nonce {
		a.log.Warn("oidc verify", "err", err, "nonce_ok", err == nil && idt.Nonce == nonce)
		http.Error(w, "Sign-in failed. Go back and try again.", http.StatusBadRequest)
		return
	}
	var c Claims
	if err := idt.Claims(&c); err != nil {
		a.log.Warn("oidc claims", "err", err)
		http.Error(w, "Sign-in failed.", http.StatusBadRequest)
		return
	}
	u, err := a.store.SignInOIDC(ctx, a.identity(&c))
	if err != nil {
		a.log.Error("sign in user", "err", err)
		http.Error(w, "Something went wrong signing you in.", http.StatusInternalServerError)
		return
	}
	a.log.Info("signed in", "user_id", u.ID, "admin", u.IsAdmin, "groups_claim", c.Groups != nil, "email_verified", c.EmailVerified != nil && *c.EmailVerified)
	returnTo := a.popReturnTo(ctx)
	if err := a.SignIn(ctx, u); err != nil {
		a.serverError(w, err)
		return
	}
	http.Redirect(w, r, returnTo, http.StatusSeeOther)
}

// identity turns verified claims into the store's sign-in input.
func (a *Auth) identity(c *Claims) store.OIDCIdentity {
	name := strings.TrimSpace(c.Name)
	if name == "" {
		name = c.PreferredUsername
	}
	if name == "" {
		name, _, _ = strings.Cut(c.Email, "@")
	}
	if name == "" {
		name = "Someone"
	}
	return store.OIDCIdentity{
		Subject:       c.Subject,
		Email:         c.Email,
		EmailVerified: c.EmailVerified != nil && *c.EmailVerified,
		Name:          name,
		IsAdmin:       a.cfg.AdminGroup != "" && slices.Contains(c.Groups, a.cfg.AdminGroup),
	}
}

// Logout ends the session. It does not sign the user out of Authentik.
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if err := a.Sessions.Destroy(r.Context()); err != nil {
		a.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/auth/signed-out", http.StatusSeeOther)
}

func (a *Auth) popReturnTo(ctx context.Context) string {
	to := a.Sessions.PopString(ctx, keyReturnTo)
	if !strings.HasPrefix(to, "/") || strings.HasPrefix(to, "//") || strings.HasPrefix(to, "/\\") || strings.HasPrefix(to, "/auth/") {
		return "/"
	}
	return to
}

func (a *Auth) serverError(w http.ResponseWriter, err error) {
	a.log.Error("auth", "err", err)
	http.Error(w, "Something went wrong.", http.StatusInternalServerError)
}

// DevMode reports whether DEV_USER sign-in is in use.
func (a *Auth) DevMode() bool { return a.oidc == nil }
