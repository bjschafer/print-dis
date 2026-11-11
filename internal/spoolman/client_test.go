package spoolman

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const filamentJSON = `[
 {"id":2,"name":"Galaxy Black","material":"pla","color_hex":"1A1A1AFF","vendor":{"id":1,"name":"Prusament"}},
 {"id":1,"name":"Blue","material":"PETG","color_hex":"0000ff","vendor":{"id":2,"name":"Elegoo"}},
 {"id":3,"name":"Rainbow","material":"PLA","multi_color_hexes":"ff0000,00ff00","color_hex":"","vendor":{"id":2,"name":"Elegoo"}},
 {"id":4,"name":"Archived Only","material":"PLA","color_hex":"zzz","vendor":{"id":1,"name":"Prusament"}}
]`

const spoolJSON = `[
 {"id":1,"filament":{"id":2},"remaining_weight":500,"archived":false},
 {"id":2,"filament":{"id":1},"remaining_weight":0,"archived":false},
 {"id":3,"filament":{"id":4},"remaining_weight":900,"archived":true},
 {"id":4,"filament":{"id":3},"remaining_weight":10.5,"archived":false}
]`

type fake struct {
	srv  *httptest.Server
	hits atomic.Int32
	fail atomic.Bool
}

func newFake(t *testing.T) *fake {
	t.Helper()
	f := &fake{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		if r.Method != http.MethodGet {
			t.Errorf("non-GET request: %s", r.Method)
		}
		if f.fail.Load() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		switch r.URL.Path {
		case "/api/v1/filament":
			_, _ = w.Write([]byte(filamentJSON))
		case "/api/v1/spool":
			_, _ = w.Write([]byte(spoolJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func TestFilaments(t *testing.T) {
	f := newFake(t)
	c := New(f.srv.URL + "/")
	opts, err := c.Filaments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 4 {
		t.Fatalf("got %d options", len(opts))
	}
	byID := map[int]FilamentOption{}
	for _, o := range opts {
		byID[o.ID] = o
	}
	tests := []struct {
		id       int
		label    string
		material string
		hex      string
		stock    bool
	}{
		{2, "Prusament Galaxy Black", "PLA", "#1a1a1a", true},
		{1, "Elegoo Blue", "PETG", "#0000ff", false},
		{3, "Elegoo Rainbow", "PLA", "#ff0000", true},
		{4, "Prusament Archived Only", "PLA", "", false},
	}
	for _, tc := range tests {
		o := byID[tc.id]
		if o.Label != tc.label || o.Material != tc.material || o.ColorHex != tc.hex || o.InStock != tc.stock {
			t.Errorf("filament %d = %+v", tc.id, o)
		}
	}
	groups := GroupByMaterial(opts)
	if len(groups) != 2 || groups[0].Material != "PETG" || groups[1].Material != "PLA" || len(groups[1].Options) != 3 {
		t.Fatalf("groups = %+v", groups)
	}
	if groups[1].Options[0].Label != "Elegoo Rainbow" {
		t.Errorf("group not sorted by label: %+v", groups[1].Options)
	}
}

func TestNormalizeHex(t *testing.T) {
	for in, want := range map[string]string{
		"FF8800": "#ff8800", "#ff880080": "#ff8800", " 00ff00 ": "#00ff00",
		"": "", "fff": "", "gggggg": "",
	} {
		if got := NormalizeHex(in); got != want {
			t.Errorf("NormalizeHex(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCache(t *testing.T) {
	f := newFake(t)
	c := New(f.srv.URL)
	now := time.Now()
	c.now = func() time.Time { return now }

	if _, err := c.Filaments(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := f.hits.Load()
	if first != 2 {
		t.Fatalf("hits = %d, want 2", first)
	}
	now = now.Add(4 * time.Minute)
	if _, err := c.Filaments(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.hits.Load() != first {
		t.Errorf("fetched again within TTL")
	}

	now = now.Add(2 * time.Minute)
	if _, err := c.Filaments(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.hits.Load() <= first {
		t.Errorf("did not refetch after TTL")
	}

	// A failed refetch serves stale data.
	f.fail.Store(true)
	now = now.Add(10 * time.Minute)
	opts, err := c.Filaments(context.Background())
	if err != nil || len(opts) != 4 {
		t.Errorf("stale: %d options, err %v", len(opts), err)
	}
}

func TestNoDataError(t *testing.T) {
	f := newFake(t)
	f.fail.Store(true)
	c := New(f.srv.URL)
	if opts, err := c.Filaments(context.Background()); err == nil {
		t.Errorf("want error, got %d options", len(opts))
	}

	f.srv.Close()
	if _, err := New(f.srv.URL).Filaments(context.Background()); err == nil {
		t.Error("want error for unreachable server")
	}
}

func TestBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("nope")) }))
	defer srv.Close()
	if _, err := New(srv.URL).Filaments(context.Background()); err == nil {
		t.Error("want error")
	}
}
