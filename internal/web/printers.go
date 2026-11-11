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
	// BambuddyURL is the saved (or, after a failed save, entered) address.
	BambuddyURL string
	// BambuddyKeySet says whether a key is saved; the key itself is never
	// sent to the browser.
	BambuddyKeySet bool
}

func (s *Server) printers(w http.ResponseWriter, r *http.Request) {
	if !core.CanManagePrinters(auth.UserFrom(r.Context())) {
		forbidden(w)
		return
	}
	s.renderPrinters(w, r, http.StatusOK, nil, nil)
}

func (s *Server) renderPrinters(w http.ResponseWriter, r *http.Request, status int, draft *core.Printer, errs formErrors) {
	s.renderPrintersWith(w, r, status, draft, nil, errs)
}

// renderPrintersWith is renderPrinters with the Bambuddy form's entered
// values when saving it failed.
func (s *Server) renderPrintersWith(w http.ResponseWriter, r *http.Request, status int, draft *core.Printer, set *core.Settings, errs formErrors) {
	ps, err := s.Store.ListPrinters(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	saved, err := s.Store.GetSettings(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	url := saved.BambuddyURL
	if set != nil {
		url = set.BambuddyURL
	}
	_, live := s.liveStatus(auth.UserFrom(r.Context()), ps)
	s.render(w, r, status, "printers.html", printersView{
		base: s.base(r, "Printers"), Printers: ps, Errors: errs, Draft: draft,
		LiveByID: live, BambuddyURL: url, BambuddyKeySet: saved.BambuddyAPIKey != "",
	})
}

// saveBambuddy saves the Bambuddy address and key. A blank key field keeps
// the saved key; "clear_api_key" removes it.
func (s *Server) saveBambuddy(w http.ResponseWriter, r *http.Request) {
	if !core.CanManagePrinters(auth.UserFrom(r.Context())) {
		forbidden(w)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad form.", http.StatusBadRequest)
		return
	}
	set, err := s.Store.GetSettings(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	errs := applyBambuddyForm(r, &set)
	if len(errs) > 0 {
		s.renderPrintersWith(w, r, http.StatusUnprocessableEntity, nil, &set, errs)
		return
	}
	if err := s.Store.SaveSettings(r.Context(), set); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.flash(r, "Saved the Bambuddy connection. Status updates within 15 seconds.")
	redirect(w, r, "/printers")
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
