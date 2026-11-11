package web

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strconv"

	"github.com/bjschafer/print-dis/internal/auth"
	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/printerstatus"
	"github.com/bjschafer/print-dis/internal/store"
)

// StatusSource is print-dis's read-only link to the printers: live status,
// cameras, and Bambuddy's printer list. *printerstatus.Monitor implements
// it; tests use a fake.
type StatusSource interface {
	Status(printerID int64) (printerstatus.Status, bool)
	Camera(ctx context.Context, t printerstatus.Target) (printerstatus.CameraInfo, error)
	OpenCamera(ctx context.Context, t printerstatus.Target) (*printerstatus.CameraStream, error)
	BambuddyPrinters(ctx context.Context, baseURL, apiKey string) ([]printerstatus.BambuddyPrinter, error)
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

// cameraTarget loads the {id} printer and its status target for the camera
// routes, writing the response and returning false when it cannot continue.
func (s *Server) cameraTarget(w http.ResponseWriter, r *http.Request) (*core.Printer, printerstatus.Target, bool) {
	if !core.CanSeePrinterStatus(auth.UserFrom(r.Context())) {
		forbidden(w)
		return nil, printerstatus.Target{}, false
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		notFound(w)
		return nil, printerstatus.Target{}, false
	}
	p, err := s.Store.GetPrinter(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		notFound(w)
		return nil, printerstatus.Target{}, false
	}
	if err != nil {
		s.serverError(w, r, err)
		return nil, printerstatus.Target{}, false
	}
	set, err := s.Store.GetSettings(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return nil, printerstatus.Target{}, false
	}
	t, ok := printerstatus.TargetFor(p, set)
	if !ok || s.PrinterStatus == nil {
		notFound(w)
		return nil, printerstatus.Target{}, false
	}
	return p, t, true
}

type cameraView struct {
	base
	Printer core.Printer
	Info    printerstatus.CameraInfo
	// Unavailable is set when the source could not describe a camera.
	Unavailable bool
	// Transform is the CSS that undoes the camera's mounting.
	Transform template.CSS
}

// camera shows a printer's live camera: the whole page, or for htmx and the
// board's camera dialog, just the view.
func (s *Server) camera(w http.ResponseWriter, r *http.Request) {
	p, t, ok := s.cameraTarget(w, r)
	if !ok {
		return
	}
	v := cameraView{base: s.base(r, p.Name+" camera"), Printer: *p}
	info, err := s.PrinterStatus.Camera(r.Context(), t)
	if err != nil {
		s.Log.Warn("camera info failed", "printer_id", p.ID, "err", err)
		v.Unavailable = true
	}
	v.Info, v.Transform = info, cameraTransform(info)
	if r.Header.Get("HX-Request") != "" {
		s.renderTemplate(w, r, http.StatusOK, "camera.html", "camera_view", v)
		return
	}
	s.render(w, r, http.StatusOK, "camera.html", v)
}

// cameraTransform turns a camera's mounting into a CSS transform.
func cameraTransform(c printerstatus.CameraInfo) template.CSS {
	var t template.CSS
	if c.Rotation != 0 {
		t += template.CSS(fmt.Sprintf("rotate(%ddeg) ", c.Rotation)) // #nosec G203 -- an int
	}
	if c.FlipH {
		t += "scaleX(-1) "
	}
	if c.FlipV {
		t += "scaleY(-1) "
	}
	return t
}

// cameraStream relays the printer's MJPEG stream. Credentials stay on the
// server, and the browser only ever talks to print-dis.
func (s *Server) cameraStream(w http.ResponseWriter, r *http.Request) {
	p, t, ok := s.cameraTarget(w, r)
	if !ok {
		return
	}
	cam, err := s.PrinterStatus.OpenCamera(r.Context(), t)
	if err != nil {
		if r.Context().Err() == nil {
			s.Log.Warn("camera open failed", "printer_id", p.ID, "err", err)
		}
		http.Error(w, "The camera is not available right now.", http.StatusBadGateway)
		return
	}
	defer func() { _ = cam.Body.Close() }()
	h := w.Header()
	h.Set("Content-Type", cam.ContentType)
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	buf := make([]byte, 32<<10)
	for {
		n, err := cam.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return // the viewer closed it
			}
			_ = rc.Flush()
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && r.Context().Err() == nil {
				s.Log.Info("camera stream ended", "printer_id", p.ID, "err", err)
			}
			return
		}
	}
}

// bambuddyChoices lists Bambuddy's printers for the printer form. listErr
// is true when Bambuddy is set up but could not be asked.
func (s *Server) bambuddyChoices(ctx context.Context, set core.Settings) (list []printerstatus.BambuddyPrinter, listErr bool) {
	if set.BambuddyURL == "" || s.PrinterStatus == nil {
		return nil, false
	}
	list, err := s.PrinterStatus.BambuddyPrinters(ctx, set.BambuddyURL, set.BambuddyAPIKey)
	if err != nil {
		s.Log.Warn("listing Bambuddy printers failed", "err", err)
		return nil, true
	}
	return list, false
}
