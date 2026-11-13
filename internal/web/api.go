package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/bjschafer/print-dis/internal/auth"
	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/store"
)

// The JSON API (docs/design.md section 6b, reference in docs/api.md). Every
// route needs an admin's bearer token. The session cookie is never read, so
// a browser cannot be tricked into calling it and no CSRF token is needed.

const maxAPIBody = 1 << 20

const maxKey = 300

func (s *Server) apiRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/requests", s.apiListRequests)
	mux.HandleFunc("POST /api/v1/requests", s.apiCreateRequest)
	mux.HandleFunc("GET /api/v1/requests/{id}", s.apiGetRequest)
	mux.HandleFunc("GET /api/v1/requests/{id}/files", s.apiListFiles)
	mux.HandleFunc("POST /api/v1/requests/{id}/files", s.apiUploadFile)
	mux.HandleFunc("POST /api/v1/requests/{id}/status", s.apiSetStatus)
	mux.HandleFunc("POST /api/v1/requests/{id}/printed", s.apiSetPrinted)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		apiError(w, http.StatusNotFound, "There is no such API route.")
	})
	return s.requireAPIUser(mux)
}

// requireAPIUser admits a request carrying an admin's token.
func (s *Server) requireAPIUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := s.Auth.APIUser(r)
		switch {
		case errors.Is(err, auth.ErrNoToken):
			w.Header().Set("WWW-Authenticate", "Bearer")
			apiError(w, http.StatusUnauthorized, "Send an API token in an Authorization: Bearer header. Admins create tokens on the help page.")
			return
		case errors.Is(err, store.ErrNotFound):
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			apiError(w, http.StatusUnauthorized, "That API token is not valid. It may have been revoked.")
			return
		case err != nil:
			s.apiServerError(w, r, err)
			return
		case !core.CanUseAPI(u):
			apiError(w, http.StatusForbidden, "The API is for admins only.")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), u)))
	})
}

// ---- JSON shapes ----

type apiDims struct {
	X int `json:"x"`
	Y int `json:"y"`
	Z int `json:"z"`
}

func toAPIDims(d *core.Dims) *apiDims {
	if d == nil {
		return nil
	}
	return &apiDims{d.X, d.Y, d.Z}
}

type apiRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type apiRequestJSON struct {
	ID                 int64              `json:"id"`
	URL                string             `json:"url"`
	Key                *string            `json:"key"`
	Project            string             `json:"project"`
	Title              string             `json:"title"`
	Quantity           int                `json:"quantity"`
	Printed            int                `json:"printed"`
	Status             core.Status        `json:"status"`
	Requester          apiRef             `json:"requester"`
	NeededBy           *string            `json:"needed_by"`
	Timing             core.Timing        `json:"timing"`
	Notes              string             `json:"notes"`
	SourceURL          *string            `json:"source_url"`
	Material           string             `json:"material"`
	Colors             []core.ColorChoice `json:"colors"`
	NeedsMultiMaterial bool               `json:"needs_multi_material"`
	NeedsEnclosure     bool               `json:"needs_enclosure"`
	Size               *apiDims           `json:"size_mm"`
	Printer            *apiRef            `json:"printer"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
}

func (s *Server) apiRequest(r *core.Request) apiRequestJSON {
	out := apiRequestJSON{
		ID: r.ID, URL: s.absURL(requestPath(r.ID)), Key: r.Key, Project: r.Project,
		Title: r.Title, Quantity: r.Quantity, Printed: r.Printed, Status: r.Status,
		Requester: apiRef{r.RequesterID, r.RequesterName},
		Timing:    r.Timing, Notes: r.Notes, SourceURL: r.SourceURL,
		Material: r.Material, Colors: r.Colors,
		NeedsMultiMaterial: r.NeedsMultiMaterial, NeedsEnclosure: r.NeedsEnclosure,
		Size: toAPIDims(r.SizeDims()), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if out.Colors == nil {
		out.Colors = []core.ColorChoice{}
	}
	if r.NeededBy != nil {
		d := r.NeededBy.Format(time.DateOnly)
		out.NeededBy = &d
	}
	if r.PrinterID != nil {
		out.Printer = &apiRef{*r.PrinterID, r.PrinterName}
	}
	return out
}

type apiFileJSON struct {
	ID          int64     `json:"id"`
	URL         string    `json:"url"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	SHA256      string    `json:"sha256"`
	Size        *apiDims  `json:"size_mm"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *Server) apiFile(f *core.File) apiFileJSON {
	return apiFileJSON{
		ID: f.ID, URL: s.absURL("/files/" + strconv.FormatInt(f.ID, 10)), Filename: f.Filename,
		ContentType: f.ContentType, SizeBytes: f.SizeBytes, SHA256: f.SHA256,
		Size: toAPIDims(f.Dims), CreatedAt: f.CreatedAt,
	}
}

func (s *Server) absURL(p string) string {
	return strings.TrimRight(s.Config.BaseURL.String(), "/") + p
}

// apiNewRequest is the body of POST /api/v1/requests.
type apiNewRequest struct {
	Key                *string    `json:"key"`
	Project            string     `json:"project"`
	Title              string     `json:"title"`
	Quantity           int        `json:"quantity"`
	RequesterID        int64      `json:"requester_id"`
	NeededBy           string     `json:"needed_by"`
	Timing             string     `json:"timing"`
	Notes              string     `json:"notes"`
	SourceURL          string     `json:"source_url"`
	Material           string     `json:"material"`
	Colors             []apiColor `json:"colors"`
	NeedsMultiMaterial *bool      `json:"needs_multi_material"`
	NeedsEnclosure     *bool      `json:"needs_enclosure"`
	Size               *apiDims   `json:"size_mm"`
}

// apiColor is one color asked for: at most one of a Spoolman filament, a
// palette color or an exact color, and an optional note.
type apiColor struct {
	FilamentID *int   `json:"filament_id"`
	Rough      string `json:"rough"`
	Hex        string `json:"hex"`
	Note       string `json:"note"`
}

// form encodes in as the fields the request form posts, so that the API is
// held to exactly the form's rules (applyRequestForm and applyColors),
// including that a filament's label and color come from Spoolman and never
// from the client.
func (in *apiNewRequest) form() url.Values {
	v := url.Values{}
	v.Set("title", in.Title)
	if in.Quantity != 0 {
		v.Set("quantity", strconv.Itoa(in.Quantity))
	}
	if in.RequesterID != 0 {
		v.Set("requester_id", strconv.FormatInt(in.RequesterID, 10))
	}
	v.Set("needed_by", in.NeededBy)
	v.Set("timing", in.Timing)
	v.Set("notes", in.Notes)
	v.Set("source_url", in.SourceURL)
	v.Set("material", in.Material)
	v.Set("project", in.Project)
	if d := in.Size; d != nil {
		v.Set("size_x", strconv.Itoa(d.X))
		v.Set("size_y", strconv.Itoa(d.Y))
		v.Set("size_z", strconv.Itoa(d.Z))
	}
	v.Set("colors_form", "1")
	for i, c := range in.Colors {
		k := strconv.Itoa(i)
		switch {
		case c.FilamentID != nil:
			v.Set("color_"+k, "f:"+strconv.Itoa(*c.FilamentID))
		case c.Rough != "":
			v.Set("color_"+k, "c:"+c.Rough)
		case c.Hex != "":
			v.Set("color_"+k, "hex")
			v.Set("color_hex_"+k, c.Hex)
		}
		v.Set("color_note_"+k, c.Note)
	}
	return v
}

// check finds what the form's rules cannot see in the JSON.
func (in *apiNewRequest) check() formErrors {
	var errs formErrors
	if in.Key != nil {
		k := strings.TrimSpace(*in.Key)
		switch {
		case k == "":
			in.Key = nil
		case len(k) > maxKey:
			errs.add("The key is too long (at most %d bytes).", maxKey)
		default:
			in.Key = &k
		}
	}
	if len(in.Colors) > core.MaxColors {
		errs.add("A request can list at most %d colors.", core.MaxColors)
	}
	for i, c := range in.Colors {
		kinds := 0
		if c.FilamentID != nil {
			kinds++
		}
		if c.Rough != "" {
			kinds++
		}
		if c.Hex != "" {
			kinds++
		}
		switch {
		case kinds > 1:
			errs.add("Color %d: give only one of filament_id, rough and hex.", i+1)
		case kinds == 0 && strings.TrimSpace(c.Note) == "":
			errs.add("Color %d: give a filament_id, rough, hex or note. Leave colors out for any color.", i+1)
		}
	}
	return errs
}

// ---- handlers ----

func (s *Server) apiListRequests(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.Filter{Project: q.Get("project"), Key: q.Get("key"), Sort: "created"}
	for _, raw := range q["status"] {
		st := core.Status(raw)
		if !st.Valid() {
			apiError(w, http.StatusBadRequest, fmt.Sprintf("%q is not a status. Use one of: %s.", raw, statusList()))
			return
		}
		f.Statuses = append(f.Statuses, st)
	}
	reqs, err := s.Store.ListRequests(r.Context(), f)
	if err != nil {
		s.apiServerError(w, r, err)
		return
	}
	out := make([]apiRequestJSON, len(reqs))
	for i := range reqs {
		out[i] = s.apiRequest(&reqs[i])
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) apiGetRequest(w http.ResponseWriter, r *http.Request) {
	if _, req := s.apiLoadRequest(w, r); req != nil {
		writeJSON(w, http.StatusOK, s.apiRequest(req))
	}
}

// apiCreateRequest files a request. When the body's key is already taken it
// answers 200 with that request instead, so a script can be re-run.
func (s *Server) apiCreateRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := auth.UserFrom(ctx)
	var in apiNewRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	errs := in.check()
	if in.Key != nil && len(errs) == 0 {
		if s.apiExisting(w, r, *in.Key) {
			return
		}
	}

	req := &core.Request{RequesterID: u.ID, CreatedByID: u.ID, Status: core.StatusRequested, Key: in.Key}
	fr := r.Clone(ctx)
	fr.Form = in.form()
	fr.PostForm = fr.Form
	errs = append(errs, applyRequestForm(fr, req, true)...)
	s.applyColors(fr, req, &errs)
	if in.NeedsEnclosure == nil {
		req.NeedsEnclosure = core.MaterialWantsEnclosure(req.Material)
	}
	if in.NeedsMultiMaterial == nil {
		req.NeedsMultiMaterial = len(req.Colors) > 1
	}
	if in.RequesterID != 0 {
		if _, err := s.Store.GetUser(ctx, in.RequesterID); errors.Is(err, store.ErrNotFound) {
			errs.add("There is no person with requester_id %d.", in.RequesterID)
		} else if err != nil {
			s.apiServerError(w, r, err)
			return
		}
	}
	if len(errs) > 0 {
		apiError(w, http.StatusUnprocessableEntity, strings.Join(errs, " "))
		return
	}

	err := s.Store.CreateRequest(ctx, req)
	if errors.Is(err, store.ErrDuplicate) && in.Key != nil && s.apiExisting(w, r, *in.Key) {
		return // filed by a concurrent call with the same key
	}
	if err != nil {
		s.apiServerError(w, r, err)
		return
	}
	created, err := s.Store.GetRequest(ctx, req.ID)
	if err != nil {
		s.apiServerError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/requests/"+strconv.FormatInt(created.ID, 10))
	writeJSON(w, http.StatusCreated, s.apiRequest(created))
}

// apiExisting answers with the request filed under key, reporting whether
// there was one (or whether it already wrote an error).
func (s *Server) apiExisting(w http.ResponseWriter, r *http.Request, key string) bool {
	req, err := s.Store.GetRequestByKey(r.Context(), key)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return false
	case err != nil:
		s.apiServerError(w, r, err)
		return true
	}
	writeJSON(w, http.StatusOK, s.apiRequest(req))
	return true
}

func (s *Server) apiListFiles(w http.ResponseWriter, r *http.Request) {
	_, req := s.apiLoadRequest(w, r)
	if req == nil {
		return
	}
	fs, err := s.Store.ListFiles(r.Context(), req.ID)
	if err != nil {
		s.apiServerError(w, r, err)
		return
	}
	out := make([]apiFileJSON, len(fs))
	for i := range fs {
		out[i] = s.apiFile(&fs[i])
	}
	writeJSON(w, http.StatusOK, out)
}

// apiUploadFile stores the raw body as one file named by ?name=, measuring a
// model and sizing the request from it as a browser upload does.
func (s *Server) apiUploadFile(w http.ResponseWriter, r *http.Request) {
	if s.Files == nil {
		apiError(w, http.StatusServiceUnavailable, "File uploads are turned off on this server.")
		return
	}
	u, req := s.apiLoadRequest(w, r)
	if req == nil {
		return
	}
	ctx := r.Context()
	name := sanitizeFilename(r.URL.Query().Get("name"))
	ctype, ok := fileTypes[strings.ToLower(path.Ext(name))]
	switch {
	case name == "":
		apiError(w, http.StatusBadRequest, "Name the file in the query string, as in ?name=bracket.stl.")
		return
	case !ok:
		apiError(w, http.StatusUnsupportedMediaType, "That file type is not allowed. Allowed: "+fileAccept+".")
		return
	case !core.CanUploadFile(u, req):
		apiError(w, http.StatusForbidden, "You cannot add files to this request.")
		return
	case r.ContentLength > s.Config.UploadMaxBytes:
		apiError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("The file is too large (the limit is %s).", humanBytes(s.Config.UploadMaxBytes)))
		return
	}
	n, err := s.Store.CountFiles(ctx, req.ID)
	if err != nil {
		s.apiServerError(w, r, err)
		return
	}
	if n >= maxFilesPerRequest {
		apiError(w, http.StatusConflict, fmt.Sprintf("A request can have at most %d files.", maxFilesPerRequest))
		return
	}
	f, msg, err := s.storeFile(ctx, r.Body, req.ID, u.ID, name, ctype, s.Config.UploadMaxBytes)
	if err != nil {
		s.apiServerError(w, r, err)
		return
	}
	if msg != "" {
		apiError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if f.Dims != nil {
		s.sizeFromModels(ctx, req, u, []core.Dims{*f.Dims}, []string{f.Filename})
	}
	writeJSON(w, http.StatusCreated, s.apiFile(f))
}

func (s *Server) apiSetStatus(w http.ResponseWriter, r *http.Request) {
	u, req := s.apiLoadRequest(w, r)
	if req == nil {
		return
	}
	var in struct {
		Status core.Status `json:"status"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	switch {
	case !in.Status.Valid():
		apiError(w, http.StatusUnprocessableEntity, "Give a status, one of: "+statusList()+".")
		return
	case in.Status == req.Status:
		writeJSON(w, http.StatusOK, s.apiRequest(req))
		return
	case !core.CanSetStatus(u, req, in.Status):
		apiError(w, http.StatusForbidden, "You cannot make that status change.")
		return
	}
	if err := s.Store.SetStatus(r.Context(), req.ID, in.Status, u.ID); err != nil {
		s.apiServerError(w, r, err)
		return
	}
	s.apiReload(w, r, req.ID)
}

func (s *Server) apiSetPrinted(w http.ResponseWriter, r *http.Request) {
	u, req := s.apiLoadRequest(w, r)
	if req == nil {
		return
	}
	var in struct {
		Printed *int `json:"printed"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	switch {
	case in.Printed == nil || *in.Printed < 0:
		apiError(w, http.StatusUnprocessableEntity, "Give printed as a whole number, zero or more.")
		return
	case !core.CanEditAdminFields(u):
		apiError(w, http.StatusForbidden, "You cannot change progress.")
		return
	case *in.Printed == req.Printed:
		writeJSON(w, http.StatusOK, s.apiRequest(req))
		return
	}
	req.Printed = *in.Printed
	if err := s.Store.UpdateRequest(r.Context(), req, u.ID, "changed printed"); err != nil {
		s.apiServerError(w, r, err)
		return
	}
	s.apiReload(w, r, req.ID)
}

// ---- helpers ----

// apiLoadRequest fetches the {id} request, answering 404 when there is none.
func (s *Server) apiLoadRequest(w http.ResponseWriter, r *http.Request) (*core.User, *core.Request) {
	u := auth.UserFrom(r.Context())
	id, ok := optID(r.PathValue("id"))
	if !ok {
		apiError(w, http.StatusNotFound, "There is no such request.")
		return nil, nil
	}
	req, err := s.Store.GetRequest(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		apiError(w, http.StatusNotFound, "There is no such request.")
		return nil, nil
	case err != nil:
		s.apiServerError(w, r, err)
		return nil, nil
	case !core.CanView(u, req):
		apiError(w, http.StatusForbidden, "You cannot see that request.")
		return nil, nil
	}
	return u, req
}

// apiReload answers with request id as it now stands.
func (s *Server) apiReload(w http.ResponseWriter, r *http.Request, id int64) {
	req, err := s.Store.GetRequest(r.Context(), id)
	if err != nil {
		s.apiServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.apiRequest(req))
}

// decodeJSON reads one JSON object into v, refusing unknown fields so that a
// misspelt field is an error rather than silently ignored.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAPIBody))
	dec.DisallowUnknownFields()
	err := dec.Decode(v)
	if err == nil && dec.Decode(&struct{}{}) != io.EOF {
		err = errors.New("more than one JSON value")
	}
	if err != nil {
		apiError(w, http.StatusBadRequest, "The body is not a valid JSON object for this route: "+err.Error()+".")
		return false
	}
	return true
}

func statusList() string {
	ss := make([]string, len(core.Statuses))
	for i, st := range core.Statuses {
		ss[i] = string(st)
	}
	return strings.Join(ss, ", ")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func apiError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{msg})
}

func (s *Server) apiServerError(w http.ResponseWriter, r *http.Request, err error) {
	s.Log.Error("api request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	apiError(w, http.StatusInternalServerError, "Something went wrong. Try again in a moment.")
}
