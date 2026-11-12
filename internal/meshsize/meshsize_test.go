package meshsize

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"testing"
)

// box corners spanning [0,x]×[0,y]×[0,z], as triangles (two per face is not
// needed for a bounding box; any triangles touching every corner will do).
func corners(x, y, z float32) [][3]float32 {
	var out [][3]float32
	for _, cx := range []float32{0, x} {
		for _, cy := range []float32{0, y} {
			for _, cz := range []float32{0, z} {
				out = append(out, [3]float32{cx, cy, cz})
			}
		}
	}
	return out
}

func binarySTLBytes(header string, pts [][3]float32) []byte {
	for len(pts)%3 != 0 {
		pts = append(pts, pts[0])
	}
	var b bytes.Buffer
	h := make([]byte, 80)
	copy(h, header)
	b.Write(h)
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(pts)/3))
	for i := 0; i < len(pts); i += 3 {
		_ = binary.Write(&b, binary.LittleEndian, [3]float32{}) // normal
		for _, p := range pts[i : i+3] {
			_ = binary.Write(&b, binary.LittleEndian, p)
		}
		b.Write([]byte{0, 0})
	}
	return b.Bytes()
}

func size(t *testing.T, ext string, data []byte) [3]int {
	t.Helper()
	b, err := Measure(ext, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("%s: %v", ext, err)
	}
	x, y, z := b.Size()
	return [3]int{x, y, z}
}

func TestSTL(t *testing.T) {
	pts := corners(120, 40.2, 15)
	if got := size(t, ".stl", binarySTLBytes("binary", pts)); got != [3]int{120, 41, 15} {
		t.Errorf("binary: %v", got)
	}
	// Binary files that start with "solid" are common and must not be read
	// as ASCII.
	if got := size(t, ".stl", binarySTLBytes("solid lies", pts)); got != [3]int{120, 41, 15} {
		t.Errorf("binary with solid header: %v", got)
	}
	var a strings.Builder
	a.WriteString("solid cube\n facet normal 0 0 0\n  outer loop\n")
	for _, p := range pts {
		fmt.Fprintf(&a, "   vertex %g %g %g\n", p[0]-10, p[1], p[2]+5)
	}
	a.WriteString("  endloop\n endfacet\nendsolid cube\n")
	if got := size(t, ".stl", []byte(a.String())); got != [3]int{120, 41, 15} {
		t.Errorf("ascii: %v", got)
	}
}

func TestOBJ(t *testing.T) {
	src := "# cube\nv 0 0 0\nv 10.004 0 0\nv 0 20 0\nv 0 0 -30\nvn 0 0 1\nvt 0.5 0.5\nf 1 2 3\n"
	if got := size(t, ".obj", []byte(src)); got != [3]int{10, 20, 30} {
		t.Errorf("obj: %v", got)
	}
}

func TestFailures(t *testing.T) {
	cases := map[string]struct {
		ext  string
		data []byte
		want error
	}{
		"empty ascii stl": {".stl", []byte("solid x\nendsolid\n"), ErrNoGeometry},
		"step":            {".step", []byte("ISO-10303-21;"), ErrUnsupported},
		"not a zip":       {".3mf", []byte("nope"), nil},
		"huge":            {".obj", []byte("v 0 0 0\nv 1e9 0 0\n"), nil},
	}
	for name, c := range cases {
		_, err := Measure(c.ext, bytes.NewReader(c.data), int64(len(c.data)))
		if err == nil || (c.want != nil && !errors.Is(err, c.want)) {
			t.Errorf("%s: got %v, want %v", name, err, c.want)
		}
	}
	// A truncated binary STL is not silently measured from part of the file.
	full := binarySTLBytes("x", corners(1, 1, 1))
	cut := full[:len(full)-20]
	// Fix the size check to still claim binary by passing the full length.
	if _, err := Measure(".stl", bytes.NewReader(cut), int64(len(full))); err == nil {
		t.Error("truncated binary STL measured without error")
	}
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func meshXML(id string, x, y, z float32) string {
	var s strings.Builder
	fmt.Fprintf(&s, `<object id="%s" type="model"><mesh><vertices>`, id)
	for _, p := range corners(x, y, z) {
		fmt.Fprintf(&s, `<vertex x="%g" y="%g" z="%g"/>`, p[0], p[1], p[2])
	}
	s.WriteString(`</vertices><triangles><triangle v1="0" v2="1" v3="2"/></triangles></mesh></object>`)
	return s.String()
}

const rels = `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Target="/3D/3dmodel.model" Id="rel0" Type="http://schemas.microsoft.com/3dmanufacturing/2013/01/3dmodel"/>
</Relationships>`

func TestThreeMF(t *testing.T) {
	// A plain 3MF in centimetres: one mesh, one item moved and rotated 90°
	// about Z, so X and Y swap (and are reported long side first anyway).
	plain := zipOf(t, map[string]string{
		"_rels/.rels": rels,
		"3D/3dmodel.model": `<?xml version="1.0"?><model unit="centimeter" xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02">
<resources>` + meshXML("1", 2, 5, 3) + `</resources>
<build><item objectid="1" transform="0 1 0 -1 0 0 0 0 1 100 100 0"/></build></model>`,
	})
	if got := size(t, ".3mf", plain); got != [3]int{50, 20, 30} {
		t.Errorf("plain: %v", got)
	}
	// Without io.ReaderAt the archive is buffered, but only up to a bound.
	if b, err := Measure(".3mf", io.MultiReader(bytes.NewReader(plain)), int64(len(plain))); err != nil || b.Empty() {
		t.Errorf("buffered: %v", err)
	}
	if _, err := Measure(".3mf", io.MultiReader(bytes.NewReader(plain)), maxBuffered3MF+1); err == nil {
		t.Error("buffered an archive past the bound")
	}

	// Production-extension layout as slicers write it: the root model holds
	// an assembly whose components live in another part, scaled 2× by a
	// component transform. Two items on the plate, far apart: the result is
	// a box each fits in, not the span of the plate.
	prod := zipOf(t, map[string]string{
		"_rels/.rels": rels,
		"3D/3dmodel.model": `<?xml version="1.0"?><model unit="millimeter" xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02" xmlns:p="http://schemas.microsoft.com/3dmanufacturing/production/2015/06">
<resources>
<object id="10" type="model"><components>
  <component p:path="/3D/Objects/part.model" objectid="1" transform="2 0 0 0 2 0 0 0 2 0 0 0"/>
  <component p:path="/3D/Objects/part.model" objectid="2" transform="1 0 0 0 1 0 0 0 1 0 0 40"/>
</components></object>
<object id="11" type="model"><components><component p:path="/3D/Objects/part.model" objectid="2"/></components></object>
</resources>
<build><item objectid="10" transform="1 0 0 0 1 0 0 0 1 128 128 0"/><item objectid="11" transform="1 0 0 0 1 0 0 0 1 300 0 0"/></build></model>`,
		"3D/Objects/part.model": `<?xml version="1.0"?><model unit="millimeter" xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02"><resources>` +
			meshXML("1", 10, 20, 15) + meshXML("2", 70, 5, 5) + meshXML("3", 999, 999, 999) + `</resources><build/></model>`,
	})
	// Item 10: mesh 1 scaled to 20×40×30, mesh 2 at z 40..45 → 70×40×45.
	// Item 11: 70×5×5. Unreferenced object 3 is ignored.
	if got := size(t, ".3mf", prod); got != [3]int{70, 40, 45} {
		t.Errorf("production: %v", got)
	}

	// Self-referencing components must not loop forever.
	loop := zipOf(t, map[string]string{
		"3D/3dmodel.model": `<model><resources><object id="1"><components><component objectid="1"/></components></object></resources><build><item objectid="1"/></build></model>`,
	})
	if _, err := Measure(".3mf", bytes.NewReader(loop), int64(len(loop))); err == nil {
		t.Error("component loop measured without error")
	}
}

func TestMatThen(t *testing.T) {
	scale := mat{2, 0, 0, 0, 2, 0, 0, 0, 2, 0, 0, 0}
	move := mat{1, 0, 0, 0, 1, 0, 0, 0, 1, 5, 0, 0}
	x, _, _ := scale.then(move).apply(1, 0, 0)
	if math.Abs(x-7) > 1e-9 {
		t.Errorf("scale then move: x = %v, want 7", x)
	}
	x, _, _ = move.then(scale).apply(1, 0, 0)
	if math.Abs(x-12) > 1e-9 {
		t.Errorf("move then scale: x = %v, want 12", x)
	}
}
