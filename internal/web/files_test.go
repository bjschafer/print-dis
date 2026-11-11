package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/core"
)

type upload struct {
	name string
	data []byte
}

func withFiles(dir string, maxBytes int64) func(*config.Config) {
	return func(c *config.Config) {
		c.FilesBackend, c.FilesDir = "dir", dir
		if maxBytes > 0 {
			c.UploadMaxBytes = maxBytes
		}
	}
}

// postFiles sends a multipart upload with the CSRF token in the query string.
func (h *harness) postFiles(loc string, token string, ups ...upload) (int, string) {
	h.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, u := range ups {
		w, err := mw.CreatePart(map[string][]string{
			"Content-Disposition": {mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": u.name})},
			"Content-Type":        {"text/html"}, // a lie: the server must ignore it
		})
		if err != nil {
			h.t.Fatal(err)
		}
		_, _ = w.Write(u.data)
	}
	_ = mw.Close()
	target := h.srv.URL + loc + "/files"
	if token != "" {
		target += "?csrf=" + url.QueryEscape(token)
	}
	req, _ := http.NewRequest(http.MethodPost, target, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := h.c.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// fileRequest has u file a request and returns its path.
func fileRequest(t *testing.T, hu *harness) string {
	t.Helper()
	code, loc, body := hu.post("/requests", url.Values{"title": {"Bracket"}})
	if code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", code, body)
	}
	return loc
}

func upOK(t *testing.T, h *harness, loc string, ups ...upload) {
	t.Helper()
	h.get(loc)
	if code, body := h.postFiles(loc, h.csrf, ups...); code != http.StatusSeeOther {
		t.Fatalf("upload: %d %s", code, body)
	}
}

func fileIDs(t *testing.T, h *harness, loc string) []core.File {
	t.Helper()
	fs, err := h.store.ListFiles(context.Background(), mustID(t, loc))
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func objectCount(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func TestUploadDownloadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	h := newHarness(t, withFiles(dir, 0))
	alice, bob := h.friend("Alice"), h.friend("Bob")
	ha, hb := h.as(alice), h.as(bob)
	loc := fileRequest(t, ha)

	data := bytes.Repeat([]byte("solid x\nendsolid\n"), 5000)
	upOK(t, ha, loc, upload{"bracket.stl", data})

	fs := fileIDs(t, h, loc)
	if len(fs) != 1 {
		t.Fatalf("got %d files", len(fs))
	}
	sum := sha256.Sum256(data)
	if fs[0].SHA256 != hex.EncodeToString(sum[:]) || fs[0].SizeBytes != int64(len(data)) ||
		fs[0].ContentType != "model/stl" || fs[0].Filename != "bracket.stl" || fs[0].UploaderName != "Alice" {
		t.Fatalf("row: %+v", fs[0])
	}
	if !strings.HasPrefix(fs[0].ObjectKey, fmt.Sprintf("requests/%d/", fs[0].RequestID)) || strings.Contains(fs[0].ObjectKey, "bracket") {
		t.Fatalf("key: %q", fs[0].ObjectKey)
	}

	fileURL := fmt.Sprintf("/files/%d", fs[0].ID)
	res, err := ha.c.Get(ha.srv.URL + fileURL)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	gotSum := sha256.Sum256(got)
	if res.StatusCode != 200 || gotSum != sum {
		t.Fatalf("download: %d, hash match %v", res.StatusCode, gotSum == sum)
	}
	if cd := res.Header.Get("Content-Disposition"); cd != `attachment; filename=bracket.stl` {
		t.Fatalf("Content-Disposition %q", cd)
	}
	if res.Header.Get("Content-Type") != "model/stl" || res.Header.Get("X-Content-Type-Options") != "nosniff" ||
		res.Header.Get("Content-Length") != fmt.Sprint(len(data)) {
		t.Fatalf("headers: %v", res.Header)
	}

	// The page lists it, with an event.
	_, body := ha.get(loc)
	ha.mustContain(body, "bracket.stl", fileURL, "Uploaded 1 file.", "uploaded bracket.stl", "/files/"+fmt.Sprint(fs[0].ID)+"/delete", `name="file"`)

	// Admin can download too.
	if code, _ := h.get(fileURL); code != 200 {
		t.Fatalf("admin download: %d", code)
	}

	// Bob cannot download, upload to or delete from Alice's request.
	if code, _ := hb.get(fileURL); code != http.StatusForbidden {
		t.Fatalf("bob download: %d", code)
	}
	hb.get("/")
	if code, _ := hb.postFiles(loc, hb.csrf, upload{"x.stl", []byte("x")}); code != http.StatusForbidden {
		t.Fatalf("bob upload: %d", code)
	}
	if code, _, _ := hb.post(fileURL+"/delete", nil); code != http.StatusForbidden {
		t.Fatalf("bob delete: %d", code)
	}
	if len(fileIDs(t, h, loc)) != 1 {
		t.Fatal("bob changed something")
	}
	if code, _ := ha.get("/files/99999"); code != http.StatusNotFound {
		t.Fatalf("missing file: %d", code)
	}

	// Alice removes her own upload; admin removes anyone's.
	if code, _, _ := ha.post(fileURL+"/delete", nil); code != http.StatusSeeOther {
		t.Fatalf("owner delete: %d", code)
	}
	if len(fileIDs(t, h, loc)) != 0 || objectCount(t, dir) != 0 {
		t.Fatalf("not deleted: %d objects", objectCount(t, dir))
	}
	_, body = ha.get(loc)
	ha.mustContain(body, "removed bracket.stl")

	upOK(t, ha, loc, upload{"again.3mf", []byte("zip")})
	fs = fileIDs(t, h, loc)
	if code, _, _ := h.post(fmt.Sprintf("/files/%d/delete", fs[0].ID), nil); code != http.StatusSeeOther {
		t.Fatalf("admin delete: %d", code)
	}
	if len(fileIDs(t, h, loc)) != 0 || objectCount(t, dir) != 0 {
		t.Fatal("admin delete left something")
	}
}

func TestUploadNeedsCSRFToken(t *testing.T) {
	dir := t.TempDir()
	h := newHarness(t, withFiles(dir, 0))
	loc := fileRequest(t, h)
	h.get(loc)
	if code, _ := h.postFiles(loc, "", upload{"a.stl", []byte("x")}); code != http.StatusForbidden {
		t.Fatalf("no token: %d", code)
	}
	if code, _ := h.postFiles(loc, "wrong", upload{"a.stl", []byte("x")}); code != http.StatusForbidden {
		t.Fatalf("wrong token: %d", code)
	}
	if len(fileIDs(t, h, loc)) != 0 || objectCount(t, dir) != 0 {
		t.Fatal("stored without a token")
	}
}

func TestUploadClosedOnceInProgress(t *testing.T) {
	dir := t.TempDir()
	h := newHarness(t, withFiles(dir, 0))
	ha := h.as(h.friend("Alice"))
	loc := fileRequest(t, ha)
	upOK(t, ha, loc, upload{"a.stl", []byte("x")})
	if code, _, _ := h.post(loc+"/status", url.Values{"status": {"printing"}}); code != http.StatusSeeOther {
		t.Fatalf("status: %d", code)
	}
	ha.get(loc)
	if code, _ := ha.postFiles(loc, ha.csrf, upload{"b.stl", []byte("x")}); code != http.StatusForbidden {
		t.Fatalf("upload while printing: %d", code)
	}
	id := fileIDs(t, h, loc)[0].ID
	if code, _, _ := ha.post(fmt.Sprintf("/files/%d/delete", id), nil); code != http.StatusForbidden {
		t.Fatalf("delete while printing: %d", code)
	}
	_, body := ha.get(loc)
	ha.mustNotContain(body, `name="file"`, "/delete")
	ha.mustContain(body, "a.stl")
	// Admin still can.
	upOK(t, h, loc, upload{"c.stl", []byte("x")})
	if len(fileIDs(t, h, loc)) != 2 {
		t.Fatal("admin upload failed")
	}
}

func TestUploadRejections(t *testing.T) {
	dir := t.TempDir()
	h := newHarness(t, withFiles(dir, 100))
	loc := fileRequest(t, h)

	// Extension not allowed (checked case-insensitively on the allowed ones).
	upOK(t, h, loc, upload{"evil.exe", []byte("x")}, upload{"page.html", []byte("<script>")}, upload{"NOEXT", []byte("x")})
	_, body := h.get(loc)
	h.mustContain(body, "evil.exe: that file type is not allowed")
	if len(fileIDs(t, h, loc)) != 0 || objectCount(t, dir) != 0 {
		t.Fatal("disallowed file stored")
	}
	upOK(t, h, loc, upload{"UPPER.STL", []byte("x")}, upload{"Photo.JPEG", []byte("x")})
	fs := fileIDs(t, h, loc)
	if len(fs) != 2 || fs[0].ContentType != "model/stl" || fs[1].ContentType != "image/jpeg" {
		t.Fatalf("uppercase extensions: %+v", fs)
	}

	// Over the limit: no row, no object, and the neighbours survive.
	before := objectCount(t, dir)
	upOK(t, h, loc, upload{"big.stl", bytes.Repeat([]byte("a"), 101)}, upload{"ok.stl", bytes.Repeat([]byte("a"), 100)})
	_, body = h.get(loc)
	h.mustContain(body, "big.stl: too large")
	fs = fileIDs(t, h, loc)
	if len(fs) != 3 || fs[2].Filename != "ok.stl" || objectCount(t, dir) != before+1 {
		t.Fatalf("after oversize: %d rows, %d objects (before %d)", len(fs), objectCount(t, dir), before)
	}
	for _, f := range fs {
		if f.Filename == "big.stl" {
			t.Fatal("oversize row exists")
		}
	}
	// No leftover temp files either.
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, _ error) error {
		if d != nil && strings.Contains(d.Name(), ".tmp-") {
			t.Errorf("temp file left: %s", p)
		}
		return nil
	})
}

func TestTwentyFileLimit(t *testing.T) {
	dir := t.TempDir()
	h := newHarness(t, withFiles(dir, 0))
	loc := fileRequest(t, h)
	var ups []upload
	for i := range 21 {
		ups = append(ups, upload{fmt.Sprintf("part%02d.stl", i), []byte("x")})
	}
	upOK(t, h, loc, ups...)
	if n := len(fileIDs(t, h, loc)); n != 20 || objectCount(t, dir) != 20 {
		t.Fatalf("got %d rows, %d objects", n, objectCount(t, dir))
	}
	_, body := h.get(loc)
	h.mustContain(body, "at most 20 files")
	h.mustNotContain(body, `name="file"`)
	if code, _ := h.postFiles(loc, h.csrf, upload{"more.stl", []byte("x")}); code != http.StatusSeeOther {
		t.Fatalf("22nd: %d", code)
	}
	if len(fileIDs(t, h, loc)) != 20 {
		t.Fatal("21st stored")
	}
}

func TestFilenameSanitised(t *testing.T) {
	dir := t.TempDir()
	h := newHarness(t, withFiles(dir, 0))
	loc := fileRequest(t, h)
	upOK(t, h, loc,
		upload{`../../etc/pass"wd.stl`, []byte("x")},
		upload{"C:\\Users\\me\\thing\"; x=\".3mf", []byte("x")},
		upload{"bell\x07\r\nname.stl", []byte("x")},
		upload{strings.Repeat("é", 300) + ".stl", []byte("x")},
	)
	fs := fileIDs(t, h, loc)
	if len(fs) != 4 {
		t.Fatalf("got %d files", len(fs))
	}
	if fs[0].Filename != "pass_wd.stl" {
		t.Errorf("path/quote name: %q", fs[0].Filename)
	}
	if fs[1].Filename != "thing_; x=_.3mf" {
		t.Errorf("windows path name: %q", fs[1].Filename)
	}
	if fs[2].Filename != "bellname.stl" {
		t.Errorf("control chars: %q", fs[2].Filename)
	}
	if n := len([]rune(fs[3].Filename)); n > maxFilenameRunes || !strings.HasSuffix(fs[3].Filename, ".stl") {
		t.Errorf("long name: %d runes", n)
	}
	for _, f := range fs {
		res, err := h.c.Get(fmt.Sprintf("%s/files/%d", h.srv.URL, f.ID))
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		cd := res.Header.Get("Content-Disposition")
		typ, params, err := mime.ParseMediaType(cd)
		if err != nil || typ != "attachment" || params["filename"] != f.Filename {
			t.Errorf("Content-Disposition %q: %v %v", cd, typ, err)
		}
	}
	if objectCount(t, dir) != 4 {
		t.Fatal("wrong object count")
	}
	// Nothing was written outside the files directory.
	if _, err := os.Stat(filepath.Join(dir, "..", "etc")); err == nil {
		t.Fatal("path traversal")
	}
}

func TestInlineOnlyForImages(t *testing.T) {
	h := newHarness(t, withFiles(t.TempDir(), 0))
	loc := fileRequest(t, h)
	upOK(t, h, loc, upload{"pic.png", []byte("png")}, upload{"model.stl", []byte("stl")})
	fs := fileIDs(t, h, loc)
	cd := func(id int64, q string) string {
		res, err := h.c.Get(fmt.Sprintf("%s/files/%d%s", h.srv.URL, id, q))
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		return res.Header.Get("Content-Disposition")
	}
	if got := cd(fs[0].ID, "?inline=1"); !strings.HasPrefix(got, "inline") {
		t.Errorf("image inline: %q", got)
	}
	if got := cd(fs[0].ID, ""); !strings.HasPrefix(got, "attachment") {
		t.Errorf("image default: %q", got)
	}
	if got := cd(fs[1].ID, "?inline=1"); !strings.HasPrefix(got, "attachment") {
		t.Errorf("stl inline: %q", got)
	}
	_, body := h.get(loc)
	h.mustContain(body, fmt.Sprintf(`src="/files/%d?inline=1"`, fs[0].ID))
	h.mustNotContain(body, fmt.Sprintf(`src="/files/%d?inline=1"`, fs[1].ID))
}

func TestUploadsDisabled(t *testing.T) {
	h := newHarness(t)
	loc := fileRequest(t, h)
	_, body := h.get(loc)
	h.mustNotContain(body, `name="file"`, `enctype="multipart/form-data"`)
	h.get("/")
	if code, _ := h.postFiles(loc, h.csrf, upload{"a.stl", []byte("x")}); code != http.StatusNotFound {
		t.Fatalf("upload: %d", code)
	}
	if code, _ := h.get("/files/1"); code != http.StatusNotFound {
		t.Fatalf("download: %d", code)
	}
	if code, _, _ := h.post("/files/1/delete", nil); code != http.StatusNotFound {
		t.Fatalf("delete: %d", code)
	}
}
