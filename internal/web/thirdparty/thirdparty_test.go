package thirdparty

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A Renovate bump moves a Version without touching the file it names; this
// keeps that half-done state red until `make vendor-js` has run.
func TestVendoredFilesMatchPins(t *testing.T) {
	for _, s := range Scripts {
		data, err := os.ReadFile(filepath.Join("..", "static", s.File))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(data, []byte(s.VersionMarker())) {
			t.Errorf("static/%s is not %s %s (no %q in it): run `make vendor-js`", s.File, s.Package, s.Version, s.VersionMarker())
		}
	}
}

func TestFetch(t *testing.T) {
	script := Script{Package: "lib", Version: "1.2.3", Path: "dist/lib.min.js", File: "lib.min.js", Marker: "lib v%s"}
	good := []byte("/* lib v1.2.3 */ code()")

	tests := []struct {
		name      string
		file      []byte // contents of package/dist/lib.min.js
		path      string // name of the file in the tarball
		integrity func(tgz []byte) string
		wantErr   string
	}{
		{name: "ok", file: good, path: "package/dist/lib.min.js", integrity: sha512Integrity},
		{name: "tampered tarball", file: good, path: "package/dist/lib.min.js",
			integrity: func(tgz []byte) string { return sha512Integrity(append([]byte("x"), tgz...)) },
			wantErr:   "does not match"},
		{name: "sha1 only", file: good, path: "package/dist/lib.min.js",
			integrity: func([]byte) string { return "sha1-AAAA" }, wantErr: "no sha512"},
		{name: "file missing", file: good, path: "package/lib.js", integrity: sha512Integrity, wantErr: "not in tarball"},
		{name: "wrong version inside", file: []byte("/* lib v1.2.2 */"), path: "package/dist/lib.min.js",
			integrity: sha512Integrity, wantErr: "does not contain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tgz := tarball(t, tt.path, tt.file)
			mux := http.NewServeMux()
			srv := httptest.NewServer(mux)
			defer srv.Close()
			mux.HandleFunc("GET /lib/1.2.3", func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"dist": map[string]string{
					"tarball": srv.URL + "/lib/-/lib-1.2.3.tgz", "integrity": tt.integrity(tgz),
				}})
			})
			mux.HandleFunc("GET /lib/-/lib-1.2.3.tgz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tgz) })

			got, err := Fetch(context.Background(), srv.Client(), srv.URL, script)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tt.file) {
				t.Errorf("got %q, want %q", got, tt.file)
			}
		})
	}
}

func TestFetchRegistryDown(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	if _, err := Fetch(context.Background(), srv.Client(), srv.URL, Scripts[0]); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err = %v, want a 404", err)
	}
}

func sha512Integrity(b []byte) string {
	sum := sha512.Sum512(b)
	return "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
}

func tarball(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, f := range []struct {
		name string
		data []byte
	}{{"package/package.json", []byte(`{}`)}, {name, data}} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o644, Size: int64(len(f.data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(f.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
