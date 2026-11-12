package web

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/store"
)

func roughChoice(key string) core.ColorChoice {
	rc, _ := core.RoughColorByKey(key)
	return core.ColorChoice{Rough: rc.Key, Label: rc.Name, Hexes: []string{rc.Hex}}
}

func TestCloneRequest(t *testing.T) {
	dir := t.TempDir()
	h := newHarness(t, withFiles(dir, 0))
	alice := h.friend("Alice")
	ha, hb := h.as(alice), h.as(h.friend("Bob"))
	ctx := context.Background()

	link, typ := "https://example.com/m/1", "image/png"
	src := &core.Request{
		RequesterID: alice.ID, CreatedByID: alice.ID, Status: core.StatusRequested,
		Title: "Adapter", Quantity: 8, Timing: core.TimingFirm, Notes: "fixed torque",
		SourceURL: &link, SourceImage: []byte("thumb"), SourceImageType: &typ,
		Material: "PETG", Colors: []core.ColorChoice{roughChoice("red")},
	}
	if err := h.store.CreateRequest(ctx, src); err != nil {
		t.Fatal(err)
	}
	loc := requestPath(src.ID)
	upOK(t, ha, loc, upload{"adapter.stl", cubeSTL(10, 20, 5)}, upload{"photo.png", []byte("png")})
	srcFiles := fileIDs(t, h, loc)
	from := "/requests/new?from=" + itoa(src.ID)

	// Cloning needs sight of the original, both for the form and the post.
	if code, _ := hb.get(from); code != http.StatusForbidden {
		t.Fatalf("bob's clone form: %d", code)
	}
	if code, _, _ := hb.post("/requests", url.Values{"title": {"Mine"}, "clone_from": {itoa(src.ID)}}); code != http.StatusForbidden {
		t.Fatalf("bob's clone post: %d", code)
	}

	_, body := ha.get(loc)
	ha.mustContain(body, `href="`+from+`"`)
	_, body = ha.get(from)
	ha.mustContain(body, `value="Adapter"`, "fixed torque", `value="https://example.com/m/1"`,
		`name="clone_from" value="`+itoa(src.ID)+`"`, `value="c:red" checked`, loc+"/image",
		fmt.Sprintf(`name="clone_file" value="%d" checked`, srcFiles[0].ID),
		fmt.Sprintf(`name="clone_file" value="%d" checked`, srcFiles[1].ID))

	// Change the color and count, drop the size, and copy only the model.
	code, newLoc, body := ha.post("/requests", url.Values{
		"title": {"Adapter"}, "quantity": {"6"}, "timing": {"firm"}, "notes": {"fixed torque"},
		"source_url": {link}, "material": {"PETG"}, "colors_form": {"1"}, "color_0": {"c:orange"},
		"clone_from": {itoa(src.ID)}, "clone_file": {itoa(srcFiles[0].ID)},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("clone: %d %s", code, body)
	}
	c, err := h.store.GetRequest(ctx, mustID(t, newLoc))
	if err != nil {
		t.Fatal(err)
	}
	if c.ID == src.ID || c.Title != "Adapter" || c.Quantity != 6 || c.Notes != "fixed torque" || c.Material != "PETG" ||
		c.RequesterID != alice.ID || len(c.Colors) != 1 || c.Colors[0].Rough != "orange" {
		t.Fatalf("clone: %+v", c)
	}
	if img, _, err := h.store.GetRequestImage(ctx, c.ID); err != nil || !bytes.Equal(img, []byte("thumb")) {
		t.Fatalf("thumbnail: %q %v", img, err)
	}
	// The copied model brings its measured size, which sizes the clone.
	if d := c.SizeDims(); d == nil || *d != (core.Dims{X: 20, Y: 10, Z: 5}) {
		t.Fatalf("size: %+v", d)
	}
	fs := fileIDs(t, h, newLoc)
	if len(fs) != 1 || fs[0].Filename != "adapter.stl" || fs[0].SHA256 != srcFiles[0].SHA256 ||
		fs[0].ObjectKey == srcFiles[0].ObjectKey || fs[0].Dims == nil {
		t.Fatalf("copied files: %+v", fs)
	}
	if code, body := ha.get(fmt.Sprintf("/files/%d", fs[0].ID)); code != http.StatusOK || body != string(cubeSTL(10, 20, 5)) {
		t.Fatalf("download copy: %d", code)
	}
	if n := objectCount(t, dir); n != 3 {
		t.Fatalf("objects: %d", n)
	}
	if len(fileIDs(t, h, loc)) != 2 {
		t.Fatal("the original lost files")
	}
	_, body = ha.get(newLoc)
	ha.mustContain(body, fmt.Sprintf("Copied 1 file from #%d.", src.ID), fmt.Sprintf("cloned this from #%d", src.ID))

	// A changed link does not take the old thumbnail.
	code, newLoc, body = ha.post("/requests", url.Values{
		"title": {"Adapter"}, "source_url": {"https://example.com/m/2"}, "clone_from": {itoa(src.ID)},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("clone with new link: %d %s", code, body)
	}
	if _, _, err := h.store.GetRequestImage(ctx, mustID(t, newLoc)); err == nil {
		t.Fatal("thumbnail copied for a different link")
	}
}

func TestSplitByColor(t *testing.T) {
	dir := t.TempDir()
	h := newHarness(t, withFiles(dir, 0))
	ctx := context.Background()

	fields := url.Values{
		"title": {"Adapter"}, "quantity": {"6"}, "material": {""}, "colors_form": {"1"},
		"color_0": {"c:red"}, "color_qty_0": {"8"}, "color_1": {"c:orange"}, "color_qty_1": {""},
		"needs_multi_material": {"on"}, "split_colors": {"on"},
	}
	bad := url.Values{}
	for k, v := range fields {
		bad[k] = v
	}
	bad["color_qty_0"] = []string{"0"}
	code, _, body := h.postNewWithFiles(bad)
	if code != http.StatusUnprocessableEntity || !strings.Contains(body, "count must be a whole number") {
		t.Fatalf("bad count: %d", code)
	}
	if !strings.Contains(body, `name="split_colors" data-split checked`) {
		t.Fatal("the split choice was not kept")
	}

	code, loc, body := h.postNewWithFiles(fields, upload{"adapter.stl", cubeSTL(10, 20, 5)})
	if code != http.StatusSeeOther || loc != "/" {
		t.Fatalf("split: %d %q %s", code, loc, body)
	}
	all, err := h.store.ListRequests(ctx, store.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	rs := slices.DeleteFunc(all, func(r core.Request) bool { return r.Title != "Adapter" })
	if len(rs) != 2 {
		t.Fatalf("requests: %d", len(rs))
	}
	for i, want := range []struct {
		color string
		qty   int
	}{{"red", 8}, {"orange", 6}} {
		r := rs[i]
		if len(r.Colors) != 1 || r.Colors[0].Rough != want.color || r.Quantity != want.qty || r.NeedsMultiMaterial {
			t.Fatalf("request %d: %+v", i, r)
		}
		if d := r.SizeDims(); d == nil || *d != (core.Dims{X: 20, Y: 10, Z: 5}) {
			t.Fatalf("request %d size: %+v", i, d)
		}
		if fs := fileIDs(t, h, requestPath(r.ID)); len(fs) != 1 || fs[0].Filename != "adapter.stl" {
			t.Fatalf("request %d files: %+v", i, fs)
		}
	}
	_, body = h.get("/")
	h.mustContain(body, "Filed 2 requests, one per color.")
}
