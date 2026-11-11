// Package web holds HTTP handlers, templates and embedded static files.
package web

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/store"
)

// Deps are the things the web layer needs.
type Deps struct {
	Config *config.Config
	Store  *store.Store
	Log    *slog.Logger
}

// Server is the application's http.Handler.
type Server struct {
	Deps
	mux *http.ServeMux
}

// New builds the handler and its routes.
func New(_ context.Context, d Deps) (*Server, error) {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	s := &Server{Deps: d, mux: http.NewServeMux()}
	s.routes()
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})
	s.mux.HandleFunc("GET /readyz", s.readyz)
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
