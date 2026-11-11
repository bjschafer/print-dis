// Package web holds HTTP handlers, templates and embedded static files.
package web

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/bjschafer/print-dis/internal/auth"
	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/files"
	"github.com/bjschafer/print-dis/internal/store"
)

//go:embed templates
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Deps are the things the web layer needs.
type Deps struct {
	Config *config.Config
	Store  *store.Store
	Log    *slog.Logger
	// Auth is built from Config and Store when nil.
	Auth *auth.Auth
	// Files stores uploads; nil disables uploads.
	Files files.Blobs
	// Now is the clock; time.Now when nil.
	Now func() time.Time
}

// Server is the application's http.Handler.
type Server struct {
	Deps
	handler http.Handler
	pages   map[string]*template.Template
}

// New builds the handler and its routes.
func New(ctx context.Context, d Deps) (*Server, error) {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Auth == nil {
		a, err := auth.New(ctx, d.Config, d.Store, d.Log)
		if err != nil {
			return nil, err
		}
		d.Auth = a
	}
	s := &Server{Deps: d}
	if err := s.parseTemplates(); err != nil {
		return nil, err
	}
	s.handler = s.routes()
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

func (s *Server) routes() http.Handler {
	public := http.NewServeMux()
	public.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	public.HandleFunc("GET /readyz", s.readyz)
	static, _ := fs.Sub(staticFS, "static")
	public.Handle("GET /static/", http.StripPrefix("/static/", cacheStatic(http.FileServerFS(static))))

	// Everything else needs a session; most of it needs a user.
	app := http.NewServeMux()
	s.authRoutes(app)

	signedIn := http.NewServeMux()
	signedIn.HandleFunc("GET /{$}", s.board)
	signedIn.HandleFunc("GET /requests", s.requestList)
	signedIn.HandleFunc("GET /requests/new", s.newRequest)
	signedIn.HandleFunc("POST /requests", s.createRequest)
	signedIn.HandleFunc("GET /requests/{id}", s.requestDetail)
	signedIn.HandleFunc("POST /requests/{id}", s.updateRequest)
	signedIn.HandleFunc("GET /requests/{id}/image", s.requestImage)
	signedIn.HandleFunc("POST /requests/{id}/status", s.setStatus)
	signedIn.HandleFunc("POST /requests/{id}/move", s.moveRequest)
	signedIn.HandleFunc("POST /requests/{id}/comments", s.addComment)
	signedIn.HandleFunc("GET /printers", s.printers)
	signedIn.HandleFunc("POST /printers", s.savePrinter)
	signedIn.HandleFunc("POST /printers/{id}", s.savePrinter)
	s.peopleRoutes(signedIn)
	s.fileRoutes(signedIn)
	app.Handle("/", s.Auth.RequireUser(signedIn))

	public.Handle("/", securityHeaders(s.Auth.Middleware(s.Auth.RequireCSRF(app))))
	return s.recoverer(public)
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.Store.Ping(ctx); err != nil {
		s.Log.Warn("readyz: database ping failed", "err", err)
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}

func cacheStatic(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		h.ServeHTTP(w, r)
	})
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Referrer-Policy", "same-origin")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.ServeHTTP(w, r)
	})
}

func (s *Server) recoverer(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(v)
				}
				s.Log.Error("panic", "path", r.URL.Path, "panic", v)
				http.Error(w, "Something went wrong.", http.StatusInternalServerError)
			}
		}()
		h.ServeHTTP(w, r)
	})
}

// ---- templates ----

// base is embedded in every page's data.
type base struct {
	Title string
	User  *core.User
	CSRF  string
	Path  string
	Flash string
	Today time.Time
}

func (s *Server) base(r *http.Request, title string) base {
	return base{
		Title: title,
		User:  auth.UserFrom(r.Context()),
		CSRF:  s.Auth.CSRFToken(r.Context()),
		Path:  r.URL.Path,
		Flash: s.Auth.Sessions.PopString(r.Context(), "flash"),
		Today: s.Now(),
	}
}

func (s *Server) flash(r *http.Request, msg string) { s.Auth.Sessions.Put(r.Context(), "flash", msg) }

func (s *Server) parseTemplates() error {
	pages, err := fs.Glob(templateFS, "templates/*.html")
	if err != nil {
		return err
	}
	s.pages = map[string]*template.Template{}
	for _, p := range pages {
		name := path.Base(p)
		if name == "layout.html" {
			continue
		}
		t, err := template.New(name).Funcs(funcs).ParseFS(templateFS, "templates/layout.html", "templates/partials/*.html", p)
		if err != nil {
			return fmt.Errorf("parse %s: %w", name, err)
		}
		s.pages[name] = t
	}
	return nil
}

// render executes page's "layout" template, or the named fragment for htmx.
func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, page string, data any) {
	s.renderTemplate(w, r, status, page, "layout", data)
}

func (s *Server) renderTemplate(w http.ResponseWriter, _ *http.Request, status int, page, tmpl string, data any) {
	t, ok := s.pages[page]
	if !ok {
		s.Log.Error("unknown template", "page", page)
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, tmpl, data); err != nil {
		s.Log.Error("render", "page", page, "template", tmpl, "err", err)
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// ---- errors ----

func (s *Server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	s.Log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	http.Error(w, "Something went wrong. Try again in a moment.", http.StatusInternalServerError)
}

func notFound(w http.ResponseWriter) { http.Error(w, "That page does not exist.", http.StatusNotFound) }

func forbidden(w http.ResponseWriter) {
	http.Error(w, "You do not have permission to do that.", http.StatusForbidden)
}

// loadRequest fetches the {id} request and checks the user may view it.
// It writes the response and returns nil when it cannot continue. Requests
// the user may not see are 403, so ids are not secret but contents are.
func (s *Server) loadRequest(w http.ResponseWriter, r *http.Request) (*core.User, *core.Request) {
	u := auth.UserFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		notFound(w)
		return nil, nil
	}
	req, err := s.Store.GetRequest(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		notFound(w)
		return nil, nil
	}
	if err != nil {
		s.serverError(w, r, err)
		return nil, nil
	}
	if !core.CanView(u, req) {
		forbidden(w)
		return nil, nil
	}
	return u, req
}

// redirect works for both plain and htmx form posts. Only local paths are
// allowed, so a crafted value can never send the user off-site.
func redirect(w http.ResponseWriter, r *http.Request, to string) {
	if !strings.HasPrefix(to, "/") || strings.HasPrefix(to, "//") || strings.HasPrefix(to, "/\\") {
		to = "/"
	}
	if r.Header.Get("HX-Request") != "" && r.Header.Get("HX-Boosted") == "" {
		w.Header().Set("HX-Redirect", to)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther) //nolint:gosec // G710: to is checked to be a local path above
}
