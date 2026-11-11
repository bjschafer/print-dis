package web

import (
	"net/http"
	"testing"
)

func TestHealth(t *testing.T) {
	h := newHarness(t)
	for _, p := range []string{"/healthz", "/readyz"} {
		if code, _ := h.get(p); code != http.StatusOK {
			t.Errorf("%s = %d", p, code)
		}
	}
	h.store.Close()
	if code, _ := h.get("/readyz"); code != http.StatusServiceUnavailable {
		t.Errorf("readyz after close = %d, want 503", code)
	}
}
