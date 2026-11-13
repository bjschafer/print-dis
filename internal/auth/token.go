package auth

import (
	"crypto/sha256"
	"errors"
	"net/http"
	"strings"

	"github.com/bjschafer/print-dis/internal/core"
)

// apiTokenPrefix starts every API token, so one is recognisable in a
// script's config or a secret scan.
const apiTokenPrefix = "pd_"

// ErrNoToken is returned by APIUser when the request carries no bearer token.
var ErrNoToken = errors.New("no bearer token")

// NewAPIToken returns a fresh token and the hash to store for it.
func NewAPIToken() (token string, hash []byte) {
	token = apiTokenPrefix + randomToken()
	return token, hashAPIToken(token)
}

func hashAPIToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// APIUser resolves the request's "Authorization: Bearer" token to its owner.
// It returns ErrNoToken without one, and store.ErrNotFound for a token that
// does not exist or was revoked. Sessions play no part: the API never reads
// the session cookie, which is why it needs no CSRF token.
func (a *Auth) APIUser(r *http.Request) (*core.User, error) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	token = strings.TrimSpace(token)
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return nil, ErrNoToken
	}
	return a.store.UserByAPIToken(r.Context(), hashAPIToken(token))
}
