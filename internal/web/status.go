package web

import (
	"net/http"

	"github.com/bjschafer/print-dis/internal/auth"
	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/printerstatus"
	"github.com/bjschafer/print-dis/internal/store"
)

// StatusSource gives the latest live status of a printer. *printerstatus.Monitor
// implements it; tests use a fake.
type StatusSource interface {
	Status(printerID int64) (printerstatus.Status, bool)
}

// liveView is one printer with its live status. Known is false when the
// printer has a source but has not been read yet.
type liveView struct {
	Printer core.Printer
	Status  printerstatus.Status
	Known   bool
}

// liveStatus returns the active printers with a status source and their
// readings, in the given order, plus the same keyed by printer id. Inactive
// printers are not polled. Both are empty when the user may not see printer
// status or there is no monitor.
func (s *Server) liveStatus(u *core.User, printers []core.Printer) ([]liveView, map[int64]liveView) {
	if s.PrinterStatus == nil || !core.CanSeePrinterStatus(u) {
		return nil, nil
	}
	var list []liveView
	byID := map[int64]liveView{}
	for _, p := range printers {
		if !p.Active || !p.HasStatusSource() {
			continue
		}
		st, ok := s.PrinterStatus.Status(p.ID)
		lv := liveView{Printer: p, Status: st, Known: ok}
		byID[p.ID] = lv
		list = append(list, lv)
	}
	return list, byID
}

// printerStatusFragment is what the board's status strip polls: the strip
// itself, plus out-of-band updates for each card on a printer that is
// printing.
func (s *Server) printerStatusFragment(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	if !core.CanSeePrinterStatus(u) {
		forbidden(w)
		return
	}
	printers, err := s.Store.ListPrinters(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	list, byID := s.liveStatus(u, printers)
	// Out-of-band updates only for printers with a card to update: htmx
	// complains about an update that matches nothing.
	var cards []liveView
	if len(byID) > 0 {
		printing, err := s.Store.ListRequests(r.Context(), store.Filter{Statuses: []core.Status{core.StatusPrinting}})
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		seen := map[int64]bool{}
		for _, rq := range printing {
			if rq.PrinterID == nil || seen[*rq.PrinterID] {
				continue
			}
			if lv, ok := byID[*rq.PrinterID]; ok {
				seen[*rq.PrinterID] = true
				cards = append(cards, lv)
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	s.renderTemplate(w, r, http.StatusOK, "board.html", "status_fragment", struct {
		Live  []liveView
		Cards []liveView
	}{list, cards})
}
