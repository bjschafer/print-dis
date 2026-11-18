// Package thirdparty pins the third-party scripts vendored into
// internal/web/static and fetches them from the npm registry.
//
// Renovate bumps a Version here (see renovate.json). It cannot fetch the file
// the new pin describes, so .github/workflows/vendor-js.yml runs
// `make vendor-js` on the Renovate branch and pushes the result back.
package thirdparty

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Registry is the npm registry the scripts are fetched from.
const Registry = "https://registry.npmjs.org"

// maxSize bounds a downloaded tarball and the file read out of it.
const maxSize = 20 << 20

type Script struct {
	Package string // npm package name
	Version string // exact version; renovate.json's custom manager rewrites it
	Path    string // file inside the package, without the tarball's "package/" prefix
	File    string // name under internal/web/static
	// Marker is a fmt pattern taking Version: a string the file contains for
	// that version and no other, so a pin can be checked against the file.
	Marker string
}

// VersionMarker is Marker filled in with Version.
func (s Script) VersionMarker() string { return fmt.Sprintf(s.Marker, s.Version) }

// Scripts are the vendored scripts. renovate.json's regex expects each entry
// on one line, starting with the Package then the Version field.
var Scripts = []Script{
	{Package: "htmx.org", Version: "2.0.4", Path: "dist/htmx.min.js", File: "htmx.min.js", Marker: `version:"%s"`},
	{Package: "sortablejs", Version: "1.15.6", Path: "Sortable.min.js", File: "Sortable.min.js", Marker: "Sortable %s - MIT"},
}

// Fetch downloads s's package from registry, checks the tarball against the
// integrity hash the registry publishes for it, and returns s.Path from inside.
func Fetch(ctx context.Context, client *http.Client, registry string, s Script) ([]byte, error) {
	body, err := get(ctx, client, registry+"/"+s.Package+"/"+s.Version)
	if err != nil {
		return nil, err
	}
	var meta struct {
		Dist struct {
			Tarball   string `json:"tarball"`
			Integrity string `json:"integrity"`
		} `json:"dist"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("%s@%s metadata: %w", s.Package, s.Version, err)
	}
	algo, sum, _ := strings.Cut(meta.Dist.Integrity, "-")
	want, err := base64.StdEncoding.DecodeString(sum)
	if algo != "sha512" || err != nil || len(want) != sha512.Size {
		return nil, fmt.Errorf("%s@%s: no sha512 integrity in registry metadata (got %q)", s.Package, s.Version, meta.Dist.Integrity)
	}

	tgz, err := get(ctx, client, meta.Dist.Tarball)
	if err != nil {
		return nil, err
	}
	if got := sha512.Sum512(tgz); !bytes.Equal(got[:], want) {
		return nil, fmt.Errorf("%s@%s: tarball does not match the registry's integrity hash", s.Package, s.Version)
	}

	data, err := extract(tgz, "package/"+s.Path)
	if err != nil {
		return nil, fmt.Errorf("%s@%s: %w", s.Package, s.Version, err)
	}
	if !bytes.Contains(data, []byte(s.VersionMarker())) {
		return nil, fmt.Errorf("%s@%s: %s does not contain %q; update its Marker in thirdparty.go", s.Package, s.Version, s.Path, s.VersionMarker())
	}
	return data, nil
}

func get(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	if len(body) > maxSize {
		return nil, fmt.Errorf("GET %s: larger than %d bytes", url, maxSize)
	}
	return body, nil
}

func extract(tgz []byte, name string) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s not in tarball", name)
		}
		if err != nil {
			return nil, err
		}
		if hdr.Name == name {
			return io.ReadAll(io.LimitReader(tr, maxSize))
		}
	}
}
