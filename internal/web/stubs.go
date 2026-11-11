package web

import "net/http"

// Filled in by M2 (sign-in, people) and M7 (files).

func (s *Server) authRoutes(_ *http.ServeMux)   {}
func (s *Server) peopleRoutes(_ *http.ServeMux) {}

func (s *Server) loadFiles(_ *http.Request, _ *detailView) error { return nil }
