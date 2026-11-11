package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/bjschafer/print-dis/internal/auth"
	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/store"
)

// filterView is the parsed filter bar shared by the board and the table.
type filterView struct {
	Query      url.Values
	Requesters []core.User
	Printers   []core.Printer
	Materials  []string
	Requester  int64
	Printer    int64
	Material   string
}

func (s *Server) filters(r *http.Request, u *core.User) (store.Filter, *filterView, error) {
	q := r.URL.Query()
	fv := &filterView{Query: q, Material: q.Get("material")}
	fv.Requester, _ = optID(q.Get("requester"))
	fv.Printer, _ = optID(q.Get("printer"))
	f := store.Filter{PrinterID: fv.Printer, Material: fv.Material, Sort: q.Get("sort"), Desc: q.Get("desc") != ""}
	if core.CanSeeAll(u) {
		f.RequesterID = fv.Requester
		users, err := s.Store.ListUsers(r.Context())
		if err != nil {
			return f, nil, err
		}
		fv.Requesters = users
	} else {
		f.RequesterID = u.ID
	}
	var err error
	if fv.Printers, err = s.Store.ListPrinters(r.Context()); err != nil {
		return f, nil, err
	}
	if fv.Materials, err = s.Store.ListMaterials(r.Context()); err != nil {
		return f, nil, err
	}
	return f, fv, nil
}

type column struct {
	Status   core.Status
	Requests []core.Request
}

func (s *Server) board(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	f, fv, err := s.filters(r, u)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	f.Statuses = core.BoardStatuses
	f.Sort = ""
	reqs, err := s.Store.ListRequests(r.Context(), f)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	cols := make([]column, len(core.BoardStatuses))
	idx := map[core.Status]int{}
	for i, st := range core.BoardStatuses {
		cols[i].Status = st
		idx[st] = i
	}
	for _, rq := range reqs {
		cols[idx[rq.Status]].Requests = append(cols[idx[rq.Status]].Requests, rq)
	}
	s.render(w, r, http.StatusOK, "board.html", struct {
		base
		Filters *filterView
		Columns []column
	}{s.base(r, "Board"), fv, cols})
}

func (s *Server) requestList(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	f, fv, err := s.filters(r, u)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if st := core.Status(r.URL.Query().Get("status")); st.Valid() {
		f.Statuses = []core.Status{st}
	}
	if f.Sort == "" {
		f.Sort, f.Desc = "created", true
	}
	reqs, err := s.Store.ListRequests(r.Context(), f)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "requests.html", struct {
		base
		Filters  *filterView
		Requests []core.Request
		Status   string
	}{s.base(r, "All requests"), fv, reqs, r.URL.Query().Get("status")})
}

// requestFormView backs the new-request form and the edit form on the detail page.
type requestFormView struct {
	base
	Request  *core.Request
	Errors   formErrors
	People   []core.User    // admins: the "for" picker
	Printers []core.Printer // admins: assignment
}

func (s *Server) formView(r *http.Request, title string, req *core.Request, errs formErrors) (*requestFormView, error) {
	u := auth.UserFrom(r.Context())
	v := &requestFormView{base: s.base(r, title), Request: req, Errors: errs}
	if core.CanFileFor(u) {
		var err error
		if v.People, err = s.Store.ListUsers(r.Context()); err != nil {
			return nil, err
		}
		if v.Printers, err = s.Store.ListPrinters(r.Context()); err != nil {
			return nil, err
		}
	}
	return v, nil
}

func (s *Server) newRequest(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	req := &core.Request{RequesterID: u.ID, Quantity: 1, Timing: core.TimingWhenever}
	if raw := strings.TrimSpace(r.URL.Query().Get("url")); raw != "" {
		if pu, err := url.Parse(raw); err == nil && (pu.Scheme == "http" || pu.Scheme == "https") {
			req.SourceURL = &raw
		}
	}
	if id, ok := optID(r.URL.Query().Get("for")); ok && core.CanFileFor(u) {
		req.RequesterID = id
	}
	v, err := s.formView(r, "New request", req, nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "new.html", v)
}

func (s *Server) createRequest(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad form.", http.StatusBadRequest)
		return
	}
	req := &core.Request{RequesterID: u.ID, CreatedByID: u.ID, Status: core.StatusRequested}
	admin := core.CanFileFor(u)
	errs := applyRequestForm(r, req, admin)
	if admin && len(errs) == 0 {
		if err := s.resolveRequester(r, req); err != nil {
			errs.add("%s", err.Error())
		}
	}
	if len(errs) > 0 {
		v, err := s.formView(r, "New request", req, errs)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		s.render(w, r, http.StatusUnprocessableEntity, "new.html", v)
		return
	}
	if err := s.Store.CreateRequest(r.Context(), req); err != nil {
		s.serverError(w, r, err)
		return
	}
	redirect(w, r, "/requests/"+strconv.FormatInt(req.ID, 10))
}

type detailView struct {
	requestFormView
	Checks         []core.PrinterCheck
	AssignedReason []string // non-empty when the assigned printer is ineligible
	Events         []core.Event
	Files          []core.File
	FilesEnabled   bool
	CanUpload      bool
}

func (s *Server) detail(w http.ResponseWriter, r *http.Request, status int, req *core.Request, errs formErrors) {
	ctx := r.Context()
	fv, err := s.formView(r, req.Title, req, errs)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	v := detailView{requestFormView: *fv}
	printers := fv.Printers
	if printers == nil {
		if printers, err = s.Store.ListPrinters(ctx); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	v.Checks = core.Check(req, printers)
	if req.PrinterID != nil {
		for _, c := range v.Checks {
			if c.Printer.ID == *req.PrinterID && !c.Eligible {
				v.AssignedReason = c.Reasons
			}
		}
	}
	if v.Events, err = s.Store.ListEvents(ctx, req.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.loadFiles(r, &v); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, status, "detail.html", v)
}

func (s *Server) requestDetail(w http.ResponseWriter, r *http.Request) {
	_, req := s.loadRequest(w, r)
	if req == nil {
		return
	}
	s.detail(w, r, http.StatusOK, req, nil)
}

func (s *Server) updateRequest(w http.ResponseWriter, r *http.Request) {
	u, req := s.loadRequest(w, r)
	if req == nil {
		return
	}
	if !core.CanEdit(u, req) {
		forbidden(w)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad form.", http.StatusBadRequest)
		return
	}
	before := *req
	errs := applyRequestForm(r, req, core.CanEditAdminFields(u))
	if len(errs) > 0 {
		s.detail(w, r, http.StatusUnprocessableEntity, req, errs)
		return
	}
	changed := core.ChangedFields(&before, req)
	if len(changed) == 0 {
		redirect(w, r, r.URL.Path)
		return
	}
	if err := s.Store.UpdateRequest(r.Context(), req, u.ID, "changed "+strings.Join(changed, ", ")); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.flash(r, "Saved.")
	redirect(w, r, r.URL.Path)
}

func (s *Server) requestImage(w http.ResponseWriter, r *http.Request) {
	_, req := s.loadRequest(w, r)
	if req == nil {
		return
	}
	img, typ, err := s.Store.GetRequestImage(r.Context(), req.ID)
	if err != nil {
		notFound(w)
		return
	}
	if !strings.HasPrefix(typ, "image/") || typ == "image/svg+xml" {
		typ = "application/octet-stream"
	}
	w.Header().Set("Content-Type", typ)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write(img)
}

func (s *Server) setStatus(w http.ResponseWriter, r *http.Request) {
	u, req := s.loadRequest(w, r)
	if req == nil {
		return
	}
	to := core.Status(r.PostFormValue("status"))
	if !core.CanSetStatus(u, req, to) {
		forbidden(w)
		return
	}
	if err := s.Store.SetStatus(r.Context(), req.ID, to, u.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	back := r.PostFormValue("back")
	if back != "/" {
		back = "/requests/" + strconv.FormatInt(req.ID, 10)
	}
	redirect(w, r, back)
}

func (s *Server) moveRequest(w http.ResponseWriter, r *http.Request) {
	u, req := s.loadRequest(w, r)
	if req == nil {
		return
	}
	to := core.Status(r.PostFormValue("status"))
	if !core.CanMove(u) || !to.Valid() {
		forbidden(w)
		return
	}
	var order []int64
	for part := range strings.SplitSeq(r.PostFormValue("order"), ",") {
		if id, ok := optID(strings.TrimSpace(part)); ok {
			order = append(order, id)
		}
	}
	if len(order) > 1000 {
		http.Error(w, "Too many cards.", http.StatusBadRequest)
		return
	}
	if err := s.Store.Move(r.Context(), req.ID, to, order, u.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

const maxComment = 5000

func (s *Server) addComment(w http.ResponseWriter, r *http.Request) {
	u, req := s.loadRequest(w, r)
	if req == nil {
		return
	}
	if !core.CanComment(u, req) {
		forbidden(w)
		return
	}
	body := strings.TrimSpace(strings.ReplaceAll(r.PostFormValue("body"), "\r\n", "\n"))
	if body == "" || len([]rune(body)) > maxComment {
		s.flash(r, "Comments must be between 1 and 5000 characters.")
		redirect(w, r, "/requests/"+strconv.FormatInt(req.ID, 10))
		return
	}
	if err := s.Store.AddEvent(r.Context(), req.ID, u.ID, core.EventComment, body); err != nil {
		s.serverError(w, r, err)
		return
	}
	if r.Header.Get("HX-Request") != "" {
		evs, err := s.Store.ListEvents(r.Context(), req.ID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		s.renderTemplate(w, r, http.StatusOK, "detail.html", "thread", struct{ Events []core.Event }{evs})
		return
	}
	redirect(w, r, "/requests/"+strconv.FormatInt(req.ID, 10)+"#thread")
}

// resolveRequester validates the admin's "for" choice, creating a
// placeholder person when a new name is given.
func (s *Server) resolveRequester(r *http.Request, req *core.Request) error {
	if name := trimmed(r, "new_person_name"); name != "" {
		email := trimmed(r, "new_person_email")
		if email != "" {
			if existing, err := s.Store.GetUserByEmail(r.Context(), email); err == nil {
				req.RequesterID = existing.ID
				return nil
			}
		}
		p, err := s.Store.CreatePlaceholder(r.Context(), name, email)
		if err != nil {
			return err
		}
		req.RequesterID = p.ID
		return nil
	}
	if req.RequesterID == 0 {
		req.RequesterID = auth.UserFrom(r.Context()).ID
	}
	if _, err := s.Store.GetUser(r.Context(), req.RequesterID); err != nil {
		return errString("Pick who the request is for.")
	}
	return nil
}

type errString string

func (e errString) Error() string { return string(e) }
