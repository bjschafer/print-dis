package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/bjschafer/print-dis/internal/auth"
	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/store"
)

type printersView struct {
	base
	Printers []core.Printer
	Errors   formErrors
	Draft    *core.Printer // the form that failed, if any
	LiveByID map[int64]liveView
	// BambuddyOn is whether BAMBUDDY_URL is set, so the form can say when a
	// Bambuddy printer id will not be used.
	BambuddyOn bool
}

func (s *Server) printers(w http.ResponseWriter, r *http.Request) {
	if !core.CanManagePrinters(auth.UserFrom(r.Context())) {
		forbidden(w)
		return
	}
	s.renderPrinters(w, r, http.StatusOK, nil, nil)
}

func (s *Server) renderPrinters(w http.ResponseWriter, r *http.Request, status int, draft *core.Printer, errs formErrors) {
	ps, err := s.Store.ListPrinters(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	_, live := s.liveStatus(auth.UserFrom(r.Context()), ps)
	s.render(w, r, status, "printers.html", printersView{
		base: s.base(r, "Printers"), Printers: ps, Errors: errs, Draft: draft,
		LiveByID: live, BambuddyOn: s.Config.BambuddyURL != "",
	})
}

func (s *Server) savePrinter(w http.ResponseWriter, r *http.Request) {
	if !core.CanManagePrinters(auth.UserFrom(r.Context())) {
		forbidden(w)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad form.", http.StatusBadRequest)
		return
	}
	p := &core.Printer{}
	if raw := r.PathValue("id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			notFound(w)
			return
		}
		p.ID = id
	}
	errs := applyPrinterForm(r, p)
	if len(errs) == 0 {
		err := s.Store.SavePrinter(r.Context(), p)
		switch {
		case errors.Is(err, store.ErrDuplicate):
			errs.add("There is already a printer called %q.", p.Name)
		case errors.Is(err, store.ErrNotFound):
			notFound(w)
			return
		case err != nil:
			s.serverError(w, r, err)
			return
		}
	}
	if len(errs) > 0 {
		s.renderPrinters(w, r, http.StatusUnprocessableEntity, p, errs)
		return
	}
	s.flash(r, "Saved "+p.Name+".")
	redirect(w, r, "/printers")
}
