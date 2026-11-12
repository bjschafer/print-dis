package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bjschafer/print-dis/internal/auth"
	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/files"
	"github.com/bjschafer/print-dis/internal/meshsize"
	"github.com/bjschafer/print-dis/internal/store"
)

// maxFilesPerRequest is the design's limit on attachments per request.
const maxFilesPerRequest = 20

// maxFilenameRunes caps the stored and displayed filename.
const maxFilenameRunes = 120

// fileTypes maps each allowed extension to the content type we store and
// serve. The client's Content-Type is never trusted.
var fileTypes = map[string]string{
	".stl":  "model/stl",
	".3mf":  "model/3mf",
	".step": "model/step",
	".stp":  "model/step",
	".obj":  "model/obj",
	".zip":  "application/zip",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
}

// inlineTypes are the only types ever served with Content-Disposition: inline.
var inlineTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true}

const fileAccept = ".stl,.3mf,.step,.stp,.obj,.zip,.png,.jpg,.jpeg,.webp"

func init() {
	funcs["isImage"] = func(f core.File) bool { return inlineTypes[f.ContentType] }
	funcs["canDeleteFile"] = core.CanDeleteFile
	funcs["fileAccept"] = func() string { return fileAccept }
	funcs["maxFiles"] = func() int { return maxFilesPerRequest }
	// sameSize reports whether the request's size is d, either way round in X and Y.
	funcs["sameSize"] = func(r *core.Request, d *core.Dims) bool {
		have := r.SizeDims()
		return have != nil && d != nil && *core.Fits(*have) == *core.Fits(*d)
	}
}

func (s *Server) fileRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /requests/{id}/files", s.uploadFiles)
	mux.HandleFunc("GET /files/{id}", s.downloadFile)
	mux.HandleFunc("POST /files/{id}/delete", s.deleteFile)
	mux.HandleFunc("POST /files/{id}/size", s.useFileSize)
}

// loadFiles fills the file section of the detail page.
func (s *Server) loadFiles(r *http.Request, v *detailView) error {
	if !v.FilesEnabled {
		return nil
	}
	fs, err := s.Store.ListFiles(r.Context(), v.Request.ID)
	if err != nil {
		return err
	}
	v.Files = fs
	v.CanUpload = core.CanUploadFile(auth.UserFrom(r.Context()), v.Request) && len(fs) < maxFilesPerRequest
	return nil
}

func requestPath(id int64) string { return "/requests/" + strconv.FormatInt(id, 10) }

// sanitizeFilename reduces a client-supplied name to something safe to store,
// show and put in a header: no directories, no control characters, no
// characters that are troublesome in file names, and a bounded length.
func sanitizeFilename(name string) string {
	name = name[strings.LastIndexAny(name, `/\`)+1:]
	if !utf8.ValidString(name) {
		name = strings.ToValidUTF8(name, "_")
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsControl(r), unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp): // Cf includes bidi overrides
			return -1
		case strings.ContainsRune(`"<>:|?*`, r):
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(name, " .")
	if rs := []rune(name); len(rs) > maxFilenameRunes {
		ext := path.Ext(name)
		if len([]rune(ext)) > 12 {
			ext = ""
		}
		keep := maxFilenameRunes - len([]rune(ext))
		name = string(rs[:keep]) + ext
	}
	return name
}

// errTooLarge is returned by limitedHasher when the limit is passed.
var errTooLarge = errors.New("file too large")

// limitedHasher hashes and counts what passes through and fails past max.
type limitedHasher struct {
	r        io.Reader
	h        hash.Hash
	n, max   int64
	exceeded bool
}

func (l *limitedHasher) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	l.n += int64(n)
	if l.n > l.max {
		l.exceeded = true
		return 0, errTooLarge
	}
	_, _ = l.h.Write(p[:n])
	return n, err
}

func randomKey(requestID int64) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "requests/" + strconv.FormatInt(requestID, 10) + "/" + hex.EncodeToString(b[:]), nil
}

func (s *Server) uploadFiles(w http.ResponseWriter, r *http.Request) {
	if s.Files == nil {
		notFound(w)
		return
	}
	u, req := s.loadRequest(w, r)
	if req == nil {
		return
	}
	if !core.CanUploadFile(u, req) {
		forbidden(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.uploadBodyLimit())
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "That was not a file upload.", http.StatusBadRequest)
		return
	}
	msg, err := s.receiveFiles(r.Context(), mr, nil, req, u)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.flash(r, msg)
	redirect(w, r, requestPath(req.ID)+"#files")
}

// uploadBodyLimit bounds a whole upload body: every file a request could
// take at the size limit, plus room for form fields and multipart overhead.
func (s *Server) uploadBodyLimit() int64 {
	return maxFilesPerRequest*s.Config.UploadMaxBytes + (1 << 20)
}

// receiveFiles stores the "file" parts of mr on req, starting with first when
// it is not nil, and returns a message saying what happened. When the files
// are models whose size can be measured and req has no size yet, it is set
// from them.
func (s *Server) receiveFiles(ctx context.Context, mr *multipart.Reader, first *multipart.Part, req *core.Request, u *core.User) (string, error) {
	existing, err := s.Store.CountFiles(ctx, req.ID)
	if err != nil {
		return "", err
	}
	room := maxFilesPerRequest - existing
	limit := s.Config.UploadMaxBytes

	var done int
	var problems []string
	var measured []core.Dims
	var measuredNames []string
	part := first
	for {
		if part == nil {
			part, err = mr.NextPart()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					problems = append(problems, "The upload was interrupted or too large.")
				}
				break
			}
		}
		if part.FormName() != "file" || part.FileName() == "" {
			_ = part.Close()
			part = nil
			continue
		}
		name := sanitizeFilename(part.FileName())
		ext := strings.ToLower(path.Ext(name))
		ctype, ok := fileTypes[ext]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s: that file type is not allowed.", name))
		case room <= 0:
			problems = append(problems, fmt.Sprintf("%s: a request can have at most %d files.", name, maxFilesPerRequest))
		default:
			f, msg, err := s.storeFile(ctx, part, req.ID, u.ID, name, ctype, limit)
			if err != nil {
				s.Log.Error("upload failed", "request", req.ID, "err", err)
				msg = name + ": the upload failed. Try again in a moment."
			}
			if msg != "" {
				problems = append(problems, msg)
			} else {
				done++
				room--
				if f.Dims != nil {
					measured = append(measured, *f.Dims)
					measuredNames = append(measuredNames, f.Filename)
				}
			}
		}
		_ = part.Close()
		part = nil
	}

	var parts []string
	if done > 0 {
		parts = append(parts, fmt.Sprintf("Uploaded %d %s.", done, plural(done, "file", "files")))
	}
	if msg := s.sizeFromModels(ctx, req, u, measured, measuredNames); msg != "" {
		parts = append(parts, msg)
	}
	return strings.Join(append(parts, problems...), " "), nil
}

// sizeFromModels sets req's size to the box all the measured models fit in
// when it has none, and returns a message when it did.
func (s *Server) sizeFromModels(ctx context.Context, req *core.Request, u *core.User, measured []core.Dims, names []string) string {
	d := core.Fits(measured...)
	if d == nil || req.HasSize() || !core.CanEdit(u, req) {
		return ""
	}
	summary := fmt.Sprintf("set size %s from %s", dimsText(*d), strings.Join(names, ", "))
	set, err := s.Store.SetRequestSize(context.WithoutCancel(ctx), req.ID, *d, true, u.ID, summary)
	if err != nil {
		s.Log.Error("setting size from models", "request", req.ID, "err", err)
		return ""
	}
	if !set {
		return ""
	}
	return fmt.Sprintf("Size set to %s from the %s.", dimsText(*d), plural(len(measured), "model", "models"))
}

func dimsText(d core.Dims) string { return fmt.Sprintf("%d × %d × %d mm", d.X, d.Y, d.Z) }

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// storeFile streams one part to the blob store and records it. It returns a
// user-facing message when the file is refused, or an error for real failures.
// Nothing is left behind in either case.
func (s *Server) storeFile(ctx context.Context, src io.Reader, requestID, userID int64, name, ctype string, limit int64) (*core.File, string, error) {
	key, err := randomKey(requestID)
	if err != nil {
		return nil, "", err
	}
	lh := &limitedHasher{r: src, h: sha256.New(), max: limit}
	cleanup := func() { s.discardObject(ctx, key) }
	if err := s.Files.Put(ctx, key, lh, -1, ctype); err != nil {
		cleanup()
		if lh.exceeded {
			return nil, fmt.Sprintf("%s: too large (the limit is %s per file).", name, humanBytes(limit)), nil
		}
		return nil, "", err
	}
	if lh.n == 0 {
		cleanup()
		return nil, name + ": the file is empty.", nil
	}
	f := &core.File{
		RequestID: requestID, UploadedByID: userID, ObjectKey: key, Filename: name,
		ContentType: ctype, SizeBytes: lh.n, SHA256: hex.EncodeToString(lh.h.Sum(nil)),
		Dims: measureFile(ctx, s.Files, s.Log, key, name, lh.n),
	}
	if err := s.Store.InsertFile(context.WithoutCancel(ctx), f); err != nil {
		cleanup()
		return nil, "", err
	}
	return f, "", nil
}

// discardObject deletes an object a failed upload or copy left behind.
func (s *Server) discardObject(ctx context.Context, key string) {
	if err := s.Files.Delete(context.WithoutCancel(ctx), key); err != nil {
		s.Log.Warn("deleting partial upload", "key", key, "err", err)
	}
}

// copyFiles copies stored files onto req as added by u, keeping their
// measured sizes, and sets req's size from them when it has none. It returns
// how many were copied, the message about the size, and a sentence for each
// failure. The 20-file limit holds without checking: req is new, and fs all
// come from one request.
func (s *Server) copyFiles(ctx context.Context, fs []core.File, req *core.Request, u *core.User) (int, string, []string) {
	var done int
	var problems, names []string
	var measured []core.Dims
	for i := range fs {
		f := &fs[i]
		if err := s.copyFile(ctx, f, req.ID, u.ID); err != nil {
			s.Log.Error("copying file", "file", f.ID, "request", req.ID, "err", err)
			problems = append(problems, fmt.Sprintf("%s could not be copied to #%d.", f.Filename, req.ID))
			continue
		}
		done++
		if f.Dims != nil {
			measured = append(measured, *f.Dims)
			names = append(names, f.Filename)
		}
	}
	return done, s.sizeFromModels(ctx, req, u, measured, names), problems
}

// copyFile streams one stored object to a new key on requestID and records
// it. Nothing is left behind on failure.
func (s *Server) copyFile(ctx context.Context, f *core.File, requestID, userID int64) error {
	key, err := randomKey(requestID)
	if err != nil {
		return err
	}
	rc, size, err := s.Files.Get(ctx, f.ObjectKey)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	if err := s.Files.Put(ctx, key, rc, size, f.ContentType); err != nil {
		s.discardObject(ctx, key)
		return err
	}
	c := &core.File{
		RequestID: requestID, UploadedByID: userID, ObjectKey: key, Filename: f.Filename,
		ContentType: f.ContentType, SizeBytes: f.SizeBytes, SHA256: f.SHA256, Dims: f.Dims,
	}
	if err := s.Store.InsertFile(context.WithoutCancel(ctx), c); err != nil {
		s.discardObject(ctx, key)
		return err
	}
	return nil
}

// measureFile reads a stored model back and returns its size, or nil when it
// is not a model, cannot be read, or holds nothing measurable. It never fails
// the upload.
func measureFile(ctx context.Context, blobs files.Blobs, log *slog.Logger, key, name string, size int64) *core.Dims {
	ext := strings.ToLower(path.Ext(name))
	if !meshsize.Supported(ext) {
		return nil
	}
	body, _, err := blobs.Get(ctx, key)
	if err != nil {
		log.Warn("reading upload to measure", "key", key, "err", err)
		return nil
	}
	defer func() { _ = body.Close() }()
	b, err := meshsize.Measure(ext, body, size)
	if err != nil {
		log.Info("could not measure model", "file", name, "err", err)
		return nil
	}
	x, y, z := b.Size()
	return &core.Dims{X: x, Y: y, Z: z}
}

// useFileSize sets a request's size from one of its measured files.
func (s *Server) useFileSize(w http.ResponseWriter, r *http.Request) {
	u, req, f := s.loadFile(w, r)
	if f == nil {
		return
	}
	if !core.CanEdit(u, req) {
		forbidden(w)
		return
	}
	if f.Dims == nil {
		http.Error(w, "That file has no measured size.", http.StatusBadRequest)
		return
	}
	summary := fmt.Sprintf("set size %s from %s", dimsText(*f.Dims), f.Filename)
	if _, err := s.Store.SetRequestSize(r.Context(), req.ID, *f.Dims, false, u.ID, summary); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.flash(r, "Size set to "+dimsText(*f.Dims)+".")
	redirect(w, r, requestPath(req.ID))
}

func humanBytes(n int64) string {
	if n >= 1<<20 {
		return fmt.Sprintf("%d MiB", n>>20)
	}
	return fmt.Sprintf("%d KiB", n>>10)
}

// loadFile fetches the {id} file and its request, and checks the user may
// view the request. It writes the response and returns nil on failure.
func (s *Server) loadFile(w http.ResponseWriter, r *http.Request) (*core.User, *core.Request, *core.File) {
	if s.Files == nil {
		notFound(w)
		return nil, nil, nil
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		notFound(w)
		return nil, nil, nil
	}
	f, err := s.Store.GetFile(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		notFound(w)
		return nil, nil, nil
	}
	if err != nil {
		s.serverError(w, r, err)
		return nil, nil, nil
	}
	req, err := s.Store.GetRequest(r.Context(), f.RequestID)
	if err != nil {
		s.serverError(w, r, err)
		return nil, nil, nil
	}
	u := auth.UserFrom(r.Context())
	if !core.CanView(u, req) {
		forbidden(w)
		return nil, nil, nil
	}
	return u, req, f
}

func (s *Server) downloadFile(w http.ResponseWriter, r *http.Request) {
	u, req, f := s.loadFile(w, r)
	if f == nil {
		return
	}
	if !core.CanDownloadFile(u, req) {
		forbidden(w)
		return
	}
	body, size, err := s.Files.Get(r.Context(), f.ObjectKey)
	if errors.Is(err, files.ErrNotFound) {
		s.Log.Error("file row without object", "file", f.ID, "key", f.ObjectKey)
		http.Error(w, "That file is missing from storage.", http.StatusNotFound)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	defer func() { _ = body.Close() }()

	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" && inlineTypes[f.ContentType] {
		disposition = "inline"
	}
	h := w.Header()
	h.Set("Content-Type", f.ContentType)
	h.Set("Content-Length", strconv.FormatInt(size, 10))
	h.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": f.Filename}))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, max-age=3600")
	if _, err := io.Copy(w, body); err != nil {
		// Headers are already sent; all we can do is log.
		s.Log.Warn("download interrupted", "file", f.ID, "err", err)
	}
}

func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request) {
	u, req, f := s.loadFile(w, r)
	if f == nil {
		return
	}
	if !core.CanDeleteFile(u, req, f) {
		forbidden(w)
		return
	}
	if err := s.Store.DeleteFile(r.Context(), f, u.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		s.serverError(w, r, err)
		return
	}
	if err := s.Files.Delete(context.WithoutCancel(r.Context()), f.ObjectKey); err != nil {
		s.Log.Error("deleting object", "file", f.ID, "key", f.ObjectKey, "err", err)
	}
	s.flash(r, "Removed "+f.Filename+".")
	redirect(w, r, requestPath(req.ID)+"#files")
}
