package web

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/bjschafer/print-dis/internal/auth"
	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/spoolman"
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
	// Projects and Project are for admins only.
	Projects []string
	Project  string
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
		fv.Project = q.Get("project")
		switch {
		case fv.Project == projectNone:
			f.NoProject = true
		case strings.HasPrefix(fv.Project, projectExcept):
			f.ExceptProject = strings.TrimPrefix(fv.Project, projectExcept)
		default:
			f.Project = fv.Project
		}
		if fv.Projects, err = s.Store.ListProjects(r.Context()); err != nil {
			return f, nil, err
		}
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
	Status    core.Status
	Requests  []core.Request
	Collapsed bool
}

// collapsedCookie lists the board columns a viewer has collapsed, as
// status names joined with ".". Script sets it; the server only reads it,
// so the board renders collapsed from the start.
const collapsedCookie = "board_collapsed"

func collapsedColumns(r *http.Request) map[core.Status]bool {
	c, err := r.Cookie(collapsedCookie)
	if err != nil {
		return nil
	}
	out := map[core.Status]bool{}
	for part := range strings.SplitSeq(c.Value, ".") {
		if st := core.Status(part); st.Valid() {
			out[st] = true
		}
	}
	return out
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
	collapsed := collapsedColumns(r)
	for i, st := range core.BoardStatuses {
		cols[i].Status = st
		cols[i].Collapsed = collapsed[st]
		idx[st] = i
	}
	for _, rq := range reqs {
		cols[idx[rq.Status]].Requests = append(cols[idx[rq.Status]].Requests, rq)
	}
	live, byID := s.liveStatus(u, fv.Printers)
	s.render(w, r, http.StatusOK, "board.html", struct {
		base
		Filters  *filterView
		Columns  []column
		Live     []liveView
		LiveByID map[int64]liveView
	}{s.base(r, "Board"), fv, cols, live, byID})
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
	Projects []string       // admins: the project field's suggestions

	// Color picker. FilamentsConfigured is true when a Spoolman source
	// exists; FilamentsDown when it is configured but could not be read,
	// in which case the form offers only rough colors.
	Filaments           []spoolman.MaterialGroup
	FilamentsConfigured bool
	FilamentsDown       bool
	InStock             int
	Materials           []string    // material chips
	MaterialOther       string      // the request's material when it is not a chip
	Slots               []colorSlot // one per color, at least one
	BlankSlot           colorSlot   // the template script copies to add a color
	extraSlots          int         // blank slots to add, for "add another color" without script
	opts                []spoolman.FilamentOption
	// Link preview state for the new-request form.
	PreviewNote string       // shown when a link could not be read
	Thumb       template.URL // data: URI of the fetched thumbnail
	FetchImage  bool         // create should fetch the thumbnail for source_url
	// File uploads, offered on the new-request form and the detail page.
	FilesEnabled bool
	UploadMax    int64
	// Cloning and splitting, on the new-request form.
	Clone    *cloneView // the request the form was copied from
	Split    bool       // file each color as its own request
	colorQty []string   // per-color counts as posted, in the order of Request.Colors
}

// cloneView is the request a new-request form was copied from.
type cloneView struct {
	ID    int64
	Title string
	Files []cloneFile
}

// cloneFile is one of its files, with whether it is ticked to be copied.
type cloneFile struct {
	core.File
	Keep bool
}

func (s *Server) formView(r *http.Request, title string, req *core.Request, errs formErrors) (*requestFormView, error) {
	u := auth.UserFrom(r.Context())
	v := &requestFormView{base: s.base(r, title), Request: req, Errors: errs,
		FilesEnabled: s.Files != nil, UploadMax: s.Config.UploadMaxBytes}
	if core.CanFileFor(u) {
		var err error
		if v.People, err = s.Store.ListUsers(r.Context()); err != nil {
			return nil, err
		}
		if v.Printers, err = s.Store.ListPrinters(r.Context()); err != nil {
			return nil, err
		}
		if v.Projects, err = s.Store.ListProjects(r.Context()); err != nil {
			return nil, err
		}
	}
	s.fillFilaments(r, v)
	return v, nil
}

// fillFilaments loads the color picker's options into v.
func (s *Server) fillFilaments(r *http.Request, v *requestFormView) {
	if s.Filaments != nil {
		v.FilamentsConfigured = true
		opts, err := s.Filaments.Filaments(r.Context())
		if err != nil {
			s.Log.Warn("filament list unavailable", "err", err)
			v.FilamentsDown = true
		} else {
			v.opts = opts
			v.Filaments = spoolman.GroupByMaterial(opts)
			for _, o := range opts {
				if o.InStock {
					v.InStock++
				}
			}
		}
	}
	v.Materials = formMaterials(v.opts)
	if m := v.Request.Material; m != "" && !slices.Contains(v.Materials, m) {
		v.MaterialOther = m
	}
	v.layoutSlots()
}

// layoutSlots (re)builds the form's color slots from the request.
func (v *requestFormView) layoutSlots() {
	v.Slots = buildSlots(v.Request.Colors, v.extraSlots, v.opts, v.FilamentsDown)
	for i := range min(len(v.Slots), len(v.colorQty)) {
		v.Slots[i].Qty = v.colorQty[i]
	}
	v.BlankSlot = colorSlot{Key: "__N__", ExactHex: defaultExactHex, ShowLabel: true}
}

// addColorRequested reports whether the form was posted by its "add
// another color" button, which without script re-shows the form with one
// more slot instead of saving.
func addColorRequested(r *http.Request) bool { return r.PostFormValue("add_color") != "" }

func (s *Server) newRequest(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	src, ok := s.loadCloneSource(w, r, r.URL.Query().Get("from"))
	if !ok {
		return
	}
	req := &core.Request{RequesterID: u.ID, Quantity: 1, Timing: core.TimingWhenever}
	if src != nil {
		req = cloneRequest(src, u)
	} else if raw := strings.TrimSpace(r.URL.Query().Get("url")); raw != "" {
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
	if src != nil {
		if v.Clone, err = s.cloneView(r.Context(), src, func(int64) bool { return true }); err != nil {
			s.serverError(w, r, err)
			return
		}
		if src.SourceImageType != nil {
			v.Thumb = template.URL(requestPath(src.ID) + "/image") // #nosec G203 -- a local path built from an id
		}
	} else if req.SourceURL != nil {
		s.prefillFromLink(r, req, v)
	}
	s.render(w, r, http.StatusOK, "new.html", v)
}

// loadCloneSource returns the request a new-request form is copied from,
// named by raw, or nil when raw is empty. When the user may not see that
// request it writes the response and reports false.
func (s *Server) loadCloneSource(w http.ResponseWriter, r *http.Request, raw string) (*core.Request, bool) {
	if raw == "" {
		return nil, true
	}
	id, ok := optID(raw)
	if !ok {
		notFound(w)
		return nil, false
	}
	src, err := s.Store.GetRequest(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		notFound(w)
		return nil, false
	}
	if err != nil {
		s.serverError(w, r, err)
		return nil, false
	}
	if !core.CanView(auth.UserFrom(r.Context()), src) {
		forbidden(w)
		return nil, false
	}
	return src, true
}

// cloneRequest is a new request asking for what src asks for. Progress,
// status and the printer start afresh; only an admin keeps its requester.
func cloneRequest(src *core.Request, u *core.User) *core.Request {
	c := &core.Request{
		RequesterID: u.ID,
		Title:       src.Title,
		Quantity:    src.Quantity,
		NeededBy:    src.NeededBy,
		Timing:      src.Timing,
		Notes:       src.Notes,
		SourceURL:   src.SourceURL,
		SourceTitle: src.SourceTitle,
		Material:    src.Material,
		Colors:      slices.Clone(src.Colors),

		NeedsMultiMaterial: src.NeedsMultiMaterial,
		NeedsEnclosure:     src.NeedsEnclosure,
		SizeX:              src.SizeX,
		SizeY:              src.SizeY,
		SizeZ:              src.SizeZ,
	}
	if core.CanFileFor(u) {
		c.RequesterID = src.RequesterID
		c.Project = src.Project
	}
	return c
}

// cloneView lists src's files for the form, ticked when keep says so.
func (s *Server) cloneView(ctx context.Context, src *core.Request, keep func(id int64) bool) (*cloneView, error) {
	v := &cloneView{ID: src.ID, Title: src.Title}
	if s.Files == nil {
		return v, nil
	}
	fs, err := s.Store.ListFiles(ctx, src.ID)
	if err != nil {
		return nil, err
	}
	for _, f := range fs {
		v.Files = append(v.Files, cloneFile{File: f, Keep: keep(f.ID)})
	}
	return v, nil
}

// splitByColor makes one request per color of req, for the new-request
// form's "file each color as its own request". A color with no count of its
// own takes req's quantity. When the form named no material, each takes its
// own filament's.
func splitByColor(req *core.Request, material string, qty []string, errs *formErrors) []*core.Request {
	out := make([]*core.Request, len(req.Colors))
	bad := false
	for i, c := range req.Colors {
		x := *req
		x.Colors = []core.ColorChoice{c}
		x.NeedsMultiMaterial = false // one color each
		if material == "" {
			x.Material = core.ColorsMaterial(x.Colors)
		}
		if i < len(qty) && qty[i] != "" {
			if n, err := strconv.Atoi(qty[i]); err != nil || n <= 0 || n > 10000 {
				bad = true
			} else {
				x.Quantity = n
			}
		}
		out[i] = &x
	}
	if bad {
		errs.add("Each color's count must be a whole number from 1 to 10000.")
	}
	return out
}

func (s *Server) createRequest(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	// With files attached the form is multipart. Its fields come first (the
	// file input is the last thing in the form) and are read here; the files
	// are streamed to storage once the request exists.
	var mr *multipart.Reader
	var firstFile *multipart.Part
	if s.Files != nil && strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, s.uploadBodyLimit())
		var err error
		if mr, err = r.MultipartReader(); err == nil {
			firstFile, err = readFields(r, mr)
		}
		if err != nil {
			http.Error(w, "Bad form.", http.StatusBadRequest)
			return
		}
	} else if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad form.", http.StatusBadRequest)
		return
	}
	src, ok := s.loadCloneSource(w, r, r.PostFormValue("clone_from"))
	if !ok {
		return
	}
	keep := map[int64]bool{}
	for _, raw := range r.PostForm["clone_file"] {
		if id, ok := optID(raw); ok {
			keep[id] = true
		}
	}
	req := &core.Request{RequesterID: u.ID, CreatedByID: u.ID, Status: core.StatusRequested}
	if src != nil {
		// So a filament Spoolman no longer lists can be kept, as on an edit.
		req.Colors = src.Colors
	}
	admin := core.CanFileFor(u)
	errs := applyRequestForm(r, req, admin)
	material := req.Material // before applyColors fills it in from the filaments
	colorQty := s.applyColors(r, req, &errs)
	reqs := []*core.Request{req}
	if checkbox(r, "split_colors") && len(req.Colors) > 1 {
		reqs = splitByColor(req, material, colorQty, &errs)
	}
	render := func(status int, errs formErrors, extraSlots int) {
		v, err := s.formView(r, "New request", req, errs)
		if err == nil && src != nil {
			v.Clone, err = s.cloneView(r.Context(), src, func(id int64) bool { return keep[id] })
		}
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		v.extraSlots, v.colorQty, v.Split = extraSlots, colorQty, checkbox(r, "split_colors")
		v.layoutSlots()
		v.FetchImage = r.PostFormValue("fetch_image") == "1"
		if src != nil && src.SourceImageType != nil {
			v.Thumb = template.URL(requestPath(src.ID) + "/image") // #nosec G203 -- a local path built from an id
		}
		s.render(w, r, status, "new.html", v)
	}
	if addColorRequested(r) {
		render(http.StatusOK, nil, 1)
		return
	}
	if admin && len(errs) == 0 {
		if err := s.resolveRequester(r, req); err != nil {
			errs.add("%s", err.Error())
		}
		for _, x := range reqs {
			x.RequesterID = req.RequesterID
		}
	}
	if len(errs) > 0 && firstFile != nil {
		errs.add("%s", "Your files were not kept; choose them again.")
	}
	if len(errs) > 0 {
		render(http.StatusUnprocessableEntity, errs, 0)
		return
	}
	if req.SourceURL != nil && r.PostFormValue("fetch_image") == "1" {
		s.attachThumbnail(r, req)
	} else if src != nil && src.SourceImageType != nil && req.SourceURL != nil && src.SourceURL != nil && *req.SourceURL == *src.SourceURL {
		if img, typ, err := s.Store.GetRequestImage(r.Context(), src.ID); err == nil {
			req.SourceImage, req.SourceImageType = img, &typ
		}
	}
	for _, x := range reqs {
		x.SourceImage, x.SourceImageType = req.SourceImage, req.SourceImageType
	}
	if err := s.Store.CreateRequest(r.Context(), reqs...); err != nil {
		s.serverError(w, r, err)
		return
	}
	if msg := s.fillNewRequests(r, mr, firstFile, src, keep, reqs, u); msg != "" {
		s.flash(r, msg)
	}
	if len(reqs) > 1 {
		redirect(w, r, "/")
		return
	}
	redirect(w, r, requestPath(req.ID))
}

// fillNewRequests finishes requests just filed from one form: it notes a
// clone's source, copies the ticked files from it, stores the uploads, and,
// when the form was split by color, copies the first request's files to the
// others. It returns what to tell the user, which may be empty.
func (s *Server) fillNewRequests(r *http.Request, mr *multipart.Reader, firstFile *multipart.Part,
	src *core.Request, keep map[int64]bool, reqs []*core.Request, u *core.User) string {
	ctx := r.Context()
	first := reqs[0]
	var msgs []string
	if len(reqs) > 1 {
		msgs = append(msgs, fmt.Sprintf("Filed %d requests, one per color.", len(reqs)))
	}
	if src != nil {
		for _, x := range reqs {
			if err := s.Store.AddEvent(ctx, x.ID, u.ID, core.EventEdit, fmt.Sprintf("cloned this from #%d", src.ID)); err != nil {
				s.Log.Error("recording clone", "request", x.ID, "err", err)
			}
		}
	}
	if s.Files == nil || !core.CanUploadFile(u, first) {
		return strings.Join(msgs, " ")
	}
	if src != nil && len(keep) > 0 {
		fs, err := s.Store.ListFiles(ctx, src.ID)
		if err != nil {
			s.Log.Error("listing files to copy", "request", src.ID, "err", err)
			msgs = append(msgs, "Files from the original could not be copied. Add them again below.")
		}
		fs = slices.DeleteFunc(fs, func(f core.File) bool { return !keep[f.ID] })
		n, sized, problems := s.copyFiles(ctx, fs, first, u)
		if n > 0 {
			msgs = append(msgs, fmt.Sprintf("Copied %d %s from #%d.", n, plural(n, "file", "files"), src.ID))
		}
		msgs = append(append(msgs, sized), problems...)
	}
	if firstFile != nil {
		msg, err := s.receiveFiles(ctx, mr, firstFile, first, u)
		if err != nil {
			s.Log.Error("uploading files with a new request", "request", first.ID, "err", err)
			msg = "The request was filed, but its files could not be saved. Add them again below."
		}
		msgs = append(msgs, msg)
	}
	if len(reqs) > 1 {
		fs, err := s.Store.ListFiles(ctx, first.ID)
		if err != nil {
			s.Log.Error("listing files to copy", "request", first.ID, "err", err)
			msgs = append(msgs, fmt.Sprintf("Files are only on #%d; they could not be copied to the others.", first.ID))
			fs = nil
		}
		if len(fs) > 0 {
			for _, x := range reqs[1:] {
				_, _, problems := s.copyFiles(ctx, fs, x, u)
				msgs = append(msgs, problems...)
			}
		}
	}
	return strings.Join(slices.DeleteFunc(msgs, func(m string) bool { return m == "" }), " ")
}

// Bounds on the plain fields of a multipart request form.
const (
	maxFieldBytes  = 64 << 10
	maxFieldsBytes = 1 << 20
)

// readFields reads the non-file parts of a multipart form into r.Form and
// r.PostForm, so the usual form helpers work, stopping at the first file
// part, which it returns (nil when there is none). A file input left empty
// arrives as a part with no file name and is skipped.
func readFields(r *http.Request, mr *multipart.Reader) (*multipart.Part, error) {
	vals := url.Values{}
	var total int64
	defer func() { r.Form, r.PostForm = vals, vals }()
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if part.FileName() != "" {
			return part, nil
		}
		b, err := io.ReadAll(io.LimitReader(part, maxFieldBytes+1))
		_ = part.Close()
		if err != nil {
			return nil, err
		}
		if total += int64(len(b)); len(b) > maxFieldBytes || total > maxFieldsBytes {
			return nil, errors.New("form field too large")
		}
		if name := part.FormName(); name != "" && name != "file" {
			vals.Add(name, string(b))
		}
	}
}

type detailView struct {
	requestFormView
	Assigned      *core.PrinterCheck  // the assigned printer, when there is one
	Checks        []core.PrinterCheck // every other printer
	OtherEligible int                 // how many of Checks are eligible
	Events        []core.Event
	Files         []core.File
	CanUpload     bool
	LiveByID      map[int64]liveView
	Shelf         []shelfMatch // admins: filaments near each rough color
}

func (s *Server) detail(w http.ResponseWriter, r *http.Request, status int, req *core.Request, errs formErrors) {
	s.detailWith(w, r, status, req, errs, 0)
}

// detailWith renders the detail page with extra blank color slots.
func (s *Server) detailWith(w http.ResponseWriter, r *http.Request, status int, req *core.Request, errs formErrors, extraSlots int) {
	ctx := r.Context()
	fv, err := s.formView(r, req.Title, req, errs)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if extraSlots > 0 {
		fv.extraSlots = extraSlots
		fv.layoutSlots()
	}
	v := detailView{requestFormView: *fv}
	if core.CanSeeAll(fv.User) {
		v.Shelf = shelfMatches(req, fv.opts)
	}
	printers := fv.Printers
	if printers == nil {
		if printers, err = s.Store.ListPrinters(ctx); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	_, v.LiveByID = s.liveStatus(fv.User, printers)
	for _, c := range core.Check(req, printers) {
		if req.PrinterID != nil && c.Printer.ID == *req.PrinterID {
			v.Assigned = &c
			continue
		}
		v.Checks = append(v.Checks, c)
		if c.Eligible {
			v.OtherEligible++
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
	s.applyColors(r, req, &errs)
	if addColorRequested(r) {
		s.detailWith(w, r, http.StatusOK, req, nil, 1)
		return
	}
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
