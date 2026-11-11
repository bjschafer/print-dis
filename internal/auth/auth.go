// Package auth handles sessions, sign-in and CSRF tokens.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"

	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/store"
)

const (
	keyUserID = "user_id"
	keyCSRF   = "csrf"
)

// Auth owns the session manager and resolves the signed-in user.
type Auth struct {
	Sessions *scs.SessionManager
	cfg      *config.Config
	store    *store.Store
	log      *slog.Logger
	oidc     *oidcClient // nil when DEV_USER is in use
}

// New builds the session manager and, unless DEV_USER is set, discovers the OIDC issuer.
func New(ctx context.Context, cfg *config.Config, st *store.Store, log *slog.Logger) (*Auth, error) {
	sm := scs.New()
	sm.Store = pgxstore.NewWithCleanupInterval(st.Pool, 30*time.Minute)
	sm.Lifetime = 30 * 24 * time.Hour
	sm.IdleTimeout = 14 * 24 * time.Hour
	sm.Cookie.Name = "print_dis_session"
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Secure = cfg.Secure()
	sm.Cookie.Persist = true

	a := &Auth{Sessions: sm, cfg: cfg, store: st, log: log}
	if cfg.DevUser == "" {
		oc, err := newOIDCClient(ctx, cfg)
		if err != nil {
			return nil, err
		}
		a.oidc = oc
	}
	return a, nil
}

type ctxKey struct{}

// UserFrom returns the signed-in user, or nil.
func UserFrom(ctx context.Context) *core.User {
	u, _ := ctx.Value(ctxKey{}).(*core.User)
	return u
}

// WithUser is for tests that call handlers directly.
func WithUser(ctx context.Context, u *core.User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// Middleware loads the session and attaches the current user, if any.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return a.Sessions.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var u *core.User
		if id := a.Sessions.GetInt64(ctx, keyUserID); id != 0 {
			got, err := a.store.GetUser(ctx, id)
			switch {
			case err == nil:
				u = got
			case errors.Is(err, store.ErrNotFound):
				// Merged away or deleted: drop the stale session.
				a.Sessions.Remove(ctx, keyUserID)
			default:
				a.log.Error("load session user", "err", err)
				http.Error(w, "Something went wrong loading your session.", http.StatusInternalServerError)
				return
			}
		}
		if u == nil && a.cfg.DevUser != "" {
			got, err := a.store.EnsureDevUser(ctx, a.cfg.DevUser)
			if err != nil {
				a.log.Error("ensure dev user", "err", err)
				http.Error(w, "Could not create the development user.", http.StatusInternalServerError)
				return
			}
			u = got
			a.Sessions.Put(ctx, keyUserID, u.ID)
		}
		if u != nil {
			ctx = WithUser(ctx, u)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	}))
}

// RequireUser redirects anonymous visitors to sign in.
func (a *Auth) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UserFrom(r.Context()) != nil {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodGet {
			a.Sessions.Put(r.Context(), keyReturnTo, r.URL.RequestURI())
		}
		if r.Header.Get("HX-Request") != "" {
			w.Header().Set("HX-Redirect", "/auth/login")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
	})
}

// CSRFToken returns this session's token, creating it if needed.
func (a *Auth) CSRFToken(ctx context.Context) string {
	if t := a.Sessions.GetString(ctx, keyCSRF); t != "" {
		return t
	}
	t := randomToken()
	a.Sessions.Put(ctx, keyCSRF, t)
	return t
}

// CSRFHeader is the header htmx and app.js send the token in.
const CSRFHeader = "X-CSRF-Token"

// CSRFField is the hidden form field name.
const CSRFField = "csrf"

// RequireCSRF rejects unsafe methods without a matching token. Cross-origin
// requests are also refused by the standard library's Sec-Fetch-Site check.
func (a *Auth) RequireCSRF(next http.Handler) http.Handler {
	cop := http.NewCrossOriginProtection()
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		want := a.Sessions.GetString(r.Context(), keyCSRF)
		got := r.Header.Get(CSRFHeader)
		if got == "" {
			if isMultipart(r) {
				// Never parse a multipart body here: uploads are streamed by
				// the handler. Such forms put the token in the query string.
				got = r.URL.Query().Get(CSRFField)
			} else {
				got = r.PostFormValue(CSRFField)
			}
		}
		if want == "" || subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
			http.Error(w, "Your session expired or the form was stale. Go back, reload and try again.", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
	return cop.Handler(inner)
}

// SignIn records u as the session's user, rotating the token to prevent fixation.
func (a *Auth) SignIn(ctx context.Context, u *core.User) error {
	if err := a.Sessions.RenewToken(ctx); err != nil {
		return err
	}
	a.Sessions.Put(ctx, keyUserID, u.ID)
	a.Sessions.Put(ctx, keyCSRF, randomToken())
	return nil
}

func isMultipart(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/")
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
