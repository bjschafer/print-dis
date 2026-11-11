package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/bjschafer/print-dis/internal/auth"
	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/store"
)

func (s *Server) authRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", s.Auth.Login)
	mux.HandleFunc("GET /auth/callback", s.Auth.Callback)
	mux.HandleFunc("POST /auth/logout", s.Auth.Logout)
	mux.HandleFunc("GET /auth/signed-out", func(w http.ResponseWriter, r *http.Request) {
		s.render(w, r, http.StatusOK, "signed_out.html", s.base(r, "Signed out"))
	})
}

func (s *Server) peopleRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /people", s.people)
	mux.HandleFunc("POST /people", s.createPerson)
	mux.HandleFunc("POST /people/merge", s.mergePeople)
}

type peopleView struct {
	base
	People []store.UserSummary
	Errors formErrors
	Name   string
	Email  string
}

func (s *Server) people(w http.ResponseWriter, r *http.Request) {
	if !core.CanManagePeople(auth.UserFrom(r.Context())) {
		forbidden(w)
		return
	}
	s.renderPeople(w, r, http.StatusOK, peopleView{})
}

func (s *Server) renderPeople(w http.ResponseWriter, r *http.Request, status int, v peopleView) {
	ps, err := s.Store.ListUserSummaries(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	v.base = s.base(r, "People")
	v.People = ps
	s.render(w, r, status, "people.html", v)
}

func (s *Server) createPerson(w http.ResponseWriter, r *http.Request) {
	if !core.CanManagePeople(auth.UserFrom(r.Context())) {
		forbidden(w)
		return
	}
	name, email := trimmed(r, "name"), trimmed(r, "email")
	var errs formErrors
	if name == "" {
		errs.add("Give the person a name.")
	}
	if email != "" && !strings.Contains(email, "@") {
		errs.add("That email address doesn't look right.")
	}
	if len(errs) == 0 {
		if _, err := s.Store.CreatePlaceholder(r.Context(), name, email); err != nil {
			errs.add("%s", err.Error())
		}
	}
	if len(errs) > 0 {
		s.renderPeople(w, r, http.StatusUnprocessableEntity, peopleView{Errors: errs, Name: name, Email: email})
		return
	}
	s.flash(r, "Added "+name+".")
	redirect(w, r, "/people")
}

func (s *Server) mergePeople(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	if !core.CanManagePeople(u) {
		forbidden(w)
		return
	}
	from, ok1 := optID(trimmed(r, "from"))
	into, ok2 := optID(trimmed(r, "into"))
	var errs formErrors
	switch {
	case !ok1 || !ok2:
		errs.add("Pick both people.")
	case from == into:
		errs.add("Pick two different people.")
	case from == u.ID:
		errs.add("You can't merge yourself away; merge the other person into you instead.")
	}
	if len(errs) == 0 {
		err := s.Store.MergeUsers(r.Context(), from, into)
		switch {
		case errors.Is(err, store.ErrMergeConflict):
			errs.add("Both of those people have signed in, so they are different accounts and can't be merged.")
		case errors.Is(err, store.ErrNotFound):
			errs.add("One of those people no longer exists.")
		case err != nil:
			s.serverError(w, r, err)
			return
		}
	}
	if len(errs) > 0 {
		s.renderPeople(w, r, http.StatusUnprocessableEntity, peopleView{Errors: errs})
		return
	}
	s.flash(r, "Merged.")
	redirect(w, r, "/people")
}
