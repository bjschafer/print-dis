package web

import (
	"errors"
	"net/http"

	"github.com/bjschafer/print-dis/internal/auth"
	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/store"
)

// newTokenKey holds a just-created token in the session until the help page
// shows it, so a reload cannot create another.
const newTokenKey = "new_api_token"

const maxTokenName = 80

func (s *Server) tokenRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /tokens", s.createToken)
	mux.HandleFunc("POST /tokens/{id}/delete", s.deleteToken)
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	if !core.CanUseAPI(u) {
		forbidden(w)
		return
	}
	name := trimmed(r, "name")
	if name == "" || len([]rune(name)) > maxTokenName {
		s.flash(r, "Give the token a name of up to 80 characters, such as what will use it.")
		redirect(w, r, "/help#api")
		return
	}
	token, hash := auth.NewAPIToken()
	if err := s.Store.CreateAPIToken(r.Context(), u.ID, name, hash); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.Auth.Sessions.Put(r.Context(), newTokenKey, token)
	redirect(w, r, "/help#api")
}

func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	if !core.CanUseAPI(u) {
		forbidden(w)
		return
	}
	id, ok := optID(r.PathValue("id"))
	if !ok {
		notFound(w)
		return
	}
	switch err := s.Store.DeleteAPIToken(r.Context(), u.ID, id); {
	case errors.Is(err, store.ErrNotFound):
		notFound(w)
		return
	case err != nil:
		s.serverError(w, r, err)
		return
	}
	s.flash(r, "Token revoked.")
	redirect(w, r, "/help#api")
}
