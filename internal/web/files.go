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
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bjschafer/print-dis/internal/auth"
	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/files"
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
}

func (s *Server) fileRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /requests/{id}/files", s.uploadFiles)
	mux.HandleFunc("GET /files/{id}", s.downloadFile)
	mux.HandleFunc("POST /files/{id}/delete", s.deleteFile)
}

// loadFiles fills the file section of the detail page.
func (s *Server) loadFiles(r *http.Request, v *detailView) error {
	v.FilesEnabled = s.Files != nil
	v.UploadMax = s.Config.UploadMaxBytes
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
	ctx := r.Context()
	existing, err := s.Store.CountFiles(ctx, req.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	room := maxFilesPerRequest - existing
	limit := s.Config.UploadMaxBytes
	// The whole body is bounded by what could legitimately be accepted plus
	// multipart overhead.
	r.Body = http.MaxBytesReader(w, r.Body, int64(max(room, 1))*limit+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "That was not a file upload.", http.StatusBadRequest)
		return
	}

	var done int
	var problems []string
	for {
		part, err := mr.NextPart()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				problems = append(problems, "The upload was interrupted or too large.")
			}
			break
		}
		if part.FormName() != "file" || part.FileName() == "" {
			_ = part.Close()
			continue
		}
		name := sanitizeFilename(part.FileName())
		ctype, ok := fileTypes[strings.ToLower(path.Ext(name))]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s: that file type is not allowed.", name))
		case room <= 0:
			problems = append(problems, fmt.Sprintf("%s: a request can have at most %d files.", name, maxFilesPerRequest))
		default:
			msg, err := s.storeFile(ctx, part, req.ID, u.ID, name, ctype, limit)
			if err != nil {
				s.Log.Error("upload failed", "request", req.ID, "err", err)
				msg = name + ": the upload failed. Try again in a moment."
			}
			if msg != "" {
				problems = append(problems, msg)
			} else {
				done++
				room--
			}
		}
		_ = part.Close()
	}

	var parts []string
	if done > 0 {
		parts = append(parts, fmt.Sprintf("Uploaded %d %s.", done, plural(done, "file", "files")))
	}
	s.flash(r, strings.Join(append(parts, problems...), " "))
	redirect(w, r, requestPath(req.ID)+"#files")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// storeFile streams one part to the blob store and records it. It returns a
// user-facing message when the file is refused, or an error for real failures.
// Nothing is left behind in either case.
func (s *Server) storeFile(ctx context.Context, src io.Reader, requestID, userID int64, name, ctype string, limit int64) (string, error) {
	key, err := randomKey(requestID)
	if err != nil {
		return "", err
	}
	lh := &limitedHasher{r: src, h: sha256.New(), max: limit}
	cleanup := func() {
		if err := s.Files.Delete(context.WithoutCancel(ctx), key); err != nil {
			s.Log.Warn("deleting partial upload", "key", key, "err", err)
		}
	}
	if err := s.Files.Put(ctx, key, lh, -1, ctype); err != nil {
		cleanup()
		if lh.exceeded {
			return fmt.Sprintf("%s: too large (the limit is %s per file).", name, humanBytes(limit)), nil
		}
		return "", err
	}
	if lh.n == 0 {
		cleanup()
		return name + ": the file is empty.", nil
	}
	f := &core.File{
		RequestID: requestID, UploadedByID: userID, ObjectKey: key, Filename: name,
		ContentType: ctype, SizeBytes: lh.n, SHA256: hex.EncodeToString(lh.h.Sum(nil)),
	}
	if err := s.Store.InsertFile(context.WithoutCancel(ctx), f); err != nil {
		cleanup()
		return "", err
	}
	return "", nil
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
