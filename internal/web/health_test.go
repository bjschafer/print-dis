package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/store/storetest"
)

func TestHealth(t *testing.T) {
	st := storetest.New(t)
	u, _ := url.Parse("http://localhost:8080")
	s, err := New(context.Background(), Deps{Config: &config.Config{BaseURL: u, DevUser: "dev@example.com"}, Store: st})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/healthz", "/readyz"} {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s = %d", p, rec.Code)
		}
	}
	st.Close()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("readyz after close = %d, want 503", rec.Code)
	}
}
