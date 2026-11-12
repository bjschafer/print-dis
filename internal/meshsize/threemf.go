package meshsize

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

// 3MF limits: decompressed reads are capped so a zip bomb cannot run the
// server out of time, and an archive without random access is buffered only
// up to a bound.
const (
	maxBuffered3MF = 64 << 20
	maxUnzipped    = 1 << 30
	maxDepth       = 16
	maxInstances   = 10_000
)

// mat is a 3MF affine transform, "m00 m01 m02 m10 m11 m12 m20 m21 m22 m30
// m31 m32", applied to row vectors: p' = p·M.
type mat [12]float64

var identity = mat{1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0}

func parseMat(s string) (mat, error) {
	if strings.TrimSpace(s) == "" {
		return identity, nil
	}
	f := strings.Fields(s)
	if len(f) != 12 {
		return mat{}, fmt.Errorf("meshsize: bad transform %q", s)
	}
	var m mat
	for i, v := range f {
		x, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return mat{}, fmt.Errorf("meshsize: bad transform %q", s)
		}
		m[i] = x
	}
	return m, nil
}

func (m mat) apply(x, y, z float64) (float64, float64, float64) {
	return x*m[0] + y*m[3] + z*m[6] + m[9],
		x*m[1] + y*m[4] + z*m[7] + m[10],
		x*m[2] + y*m[5] + z*m[8] + m[11]
}

// then returns the transform that applies m and then n.
func (m mat) then(n mat) mat {
	var o mat
	for r := range 4 { // rows of m: three linear rows and the translation
		for c := range 3 {
			v := m[r*3]*n[c] + m[r*3+1]*n[3+c] + m[r*3+2]*n[6+c]
			if r == 3 {
				v += n[9+c]
			}
			o[r*3+c] = v
		}
	}
	return o
}

type objKey struct{ part, id string }

type component struct {
	ref objKey
	m   mat
}

type object struct {
	mesh  bool
	comps []component
}

type item struct {
	ref objKey
	m   mat
}

type instance struct {
	item int // index into the build items
	m    mat
}

// threeMF measures a 3MF. Each build item (each part on the plate) is
// measured with its transforms applied, and the result is the smallest box
// every one of them fits in, X and Y taken as the longer and shorter side:
// parts laid out on a plate can be printed separately, so the plate's
// overall extent is not the request's size.
func threeMF(r io.Reader, size int64) (Box, error) {
	// A zip is read from its end, so it needs random access. Blob stores give
	// it (an S3 object reads by range, a file by offset); anything else is
	// buffered, within a bound.
	ra, ok := r.(io.ReaderAt)
	if !ok {
		if size > maxBuffered3MF {
			return Box{}, fmt.Errorf("meshsize: 3MF of %d bytes is too large to buffer", size)
		}
		buf, err := io.ReadAll(io.LimitReader(r, size+1))
		if err != nil {
			return Box{}, err
		}
		ra, size = bytes.NewReader(buf), int64(len(buf))
	}
	zr, err := zip.NewReader(ra, size)
	if err != nil {
		return Box{}, fmt.Errorf("meshsize: not a 3MF archive: %w", err)
	}
	m := &model3mf{zr: zr, objects: map[objKey]*object{}, scale: map[string]float64{}, budget: maxUnzipped}
	root := m.rootPart()
	if err := m.scan(root, true); err != nil {
		return Box{}, err
	}

	// Expand items into mesh instances, each with its full transform.
	inst := map[objKey][]instance{}
	count, visits := 0, 0
	var walk func(k objKey, mt mat, it, depth int) error
	walk = func(k objKey, mt mat, it, depth int) error {
		if depth > maxDepth {
			return errors.New("meshsize: components nest too deeply")
		}
		if visits++; visits > 10*maxInstances {
			return errors.New("meshsize: too many components")
		}
		o, err := m.object(k)
		if err != nil || o == nil {
			return err
		}
		if o.mesh {
			if count++; count > maxInstances {
				return errors.New("meshsize: too many instances")
			}
			inst[k] = append(inst[k], instance{it, mt})
		}
		for _, c := range o.comps {
			if err := walk(c.ref, c.m.then(mt), it, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	for i, it := range m.items {
		if err := walk(it.ref, it.m, i, 0); err != nil {
			return Box{}, err
		}
	}

	boxes := make([]Box, len(m.items))
	parts := map[string]bool{}
	for k := range inst {
		parts[k.part] = true
	}
	for p := range parts {
		if err := m.vertices(p, inst, boxes); err != nil {
			return Box{}, err
		}
	}

	var out Box
	for _, b := range boxes {
		if b.Empty() {
			continue
		}
		dx, dy, dz := b.Max[0]-b.Min[0], b.Max[1]-b.Min[1], b.Max[2]-b.Min[2]
		long, short := max(dx, dy), min(dx, dy)
		if out.Empty() {
			out.add(0, 0, 0)
		}
		out.Max = [3]float64{max(out.Max[0], long), max(out.Max[1], short), max(out.Max[2], dz)}
	}
	return out, nil
}

type model3mf struct {
	zr      *zip.Reader
	objects map[objKey]*object
	scanned map[string]bool
	scale   map[string]float64 // per part, to millimetres
	items   []item
	budget  int64 // decompressed bytes left
}

// rootPart finds the main model from the package relationships, falling back
// to the conventional name.
func (m *model3mf) rootPart() string {
	const fallback = "3D/3dmodel.model"
	rc, err := m.open("_rels/.rels")
	if err != nil {
		return fallback
	}
	defer func() { _ = rc.Close() }()
	var rels struct {
		Rel []struct {
			Target string `xml:"Target,attr"`
			Type   string `xml:"Type,attr"`
		} `xml:"Relationship"`
	}
	if xml.NewDecoder(rc).Decode(&rels) != nil {
		return fallback
	}
	for _, r := range rels.Rel {
		if strings.HasSuffix(r.Type, "/3dmodel") {
			return clean(r.Target)
		}
	}
	return fallback
}

func clean(p string) string { return strings.TrimPrefix(path.Clean("/"+p), "/") }

type budgetReader struct {
	r io.Reader
	m *model3mf
}

func (b budgetReader) Read(p []byte) (int, error) {
	if b.m.budget <= 0 {
		return 0, errors.New("meshsize: 3MF too large when decompressed")
	}
	if int64(len(p)) > b.m.budget {
		p = p[:b.m.budget]
	}
	n, err := b.r.Read(p)
	b.m.budget -= int64(n)
	return n, err
}

func (m *model3mf) open(name string) (io.ReadCloser, error) {
	f, err := m.zr.Open(name)
	if err != nil {
		return nil, err
	}
	return struct {
		io.Reader
		io.Closer
	}{budgetReader{f, m}, f}, nil
}

func attr(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

var unitScale = map[string]float64{
	"": 1, "millimeter": 1, "micron": 0.001, "centimeter": 10, "inch": 25.4, "foot": 304.8, "meter": 1000,
}

// object returns a parsed object, scanning its part on first use.
func (m *model3mf) object(k objKey) (*object, error) {
	if err := m.scan(k.part, false); err != nil {
		return nil, err
	}
	return m.objects[k], nil
}

// scan reads a part's object structure (not its vertices) and, for the root,
// its build items.
func (m *model3mf) scan(part string, root bool) error {
	if m.scanned == nil {
		m.scanned = map[string]bool{}
	}
	if m.scanned[part] {
		return nil
	}
	m.scanned[part] = true
	rc, err := m.open(part)
	if err != nil {
		return fmt.Errorf("meshsize: 3MF part %s: %w", part, err)
	}
	defer func() { _ = rc.Close() }()
	d := xml.NewDecoder(rc)
	var cur *object
	for {
		tok, err := d.RawToken()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("meshsize: 3MF part %s: %w", part, err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			if ee, ok := tok.(xml.EndElement); ok && ee.Name.Local == "object" {
				cur = nil
			}
			continue
		}
		switch se.Name.Local {
		case "model":
			s, ok := unitScale[attr(se, "unit")]
			if !ok {
				s = 1
			}
			m.scale[part] = s
		case "object":
			cur = &object{}
			m.objects[objKey{part, attr(se, "id")}] = cur
		case "mesh":
			if cur != nil {
				cur.mesh = true
			}
			if err := rawSkip(d); err != nil {
				return err
			}
		case "component":
			if cur == nil {
				continue
			}
			mt, err := parseMat(attr(se, "transform"))
			if err != nil {
				return err
			}
			ref := part
			if p := attr(se, "path"); p != "" {
				ref = clean(p)
			}
			cur.comps = append(cur.comps, component{objKey{ref, attr(se, "objectid")}, mt})
		case "item":
			if !root {
				continue
			}
			mt, err := parseMat(attr(se, "transform"))
			if err != nil {
				return err
			}
			m.items = append(m.items, item{objKey{part, attr(se, "objectid")}, mt})
		}
	}
}

// vertices streams a part's mesh vertices through every instance's
// transform, growing the box of the build item each instance belongs to.
func (m *model3mf) vertices(part string, inst map[objKey][]instance, boxes []Box) error {
	rc, err := m.open(part)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	scale := m.scale[part]
	if scale == 0 {
		scale = 1
	}
	d := xml.NewDecoder(rc)
	var cur []instance
	for {
		tok, err := d.RawToken()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("meshsize: 3MF part %s: %w", part, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "object":
				cur = inst[objKey{part, attr(t, "id")}]
				if cur == nil {
					if err := rawSkip(d); err != nil {
						return err
					}
				}
			case "vertex":
				x, ex := strconv.ParseFloat(attr(t, "x"), 64)
				y, ey := strconv.ParseFloat(attr(t, "y"), 64)
				z, ez := strconv.ParseFloat(attr(t, "z"), 64)
				if ex != nil || ey != nil || ez != nil {
					continue
				}
				x, y, z = x*scale, y*scale, z*scale
				for _, in := range cur {
					boxes[in.item].add(in.m.apply(x, y, z))
				}
			case "triangles":
				if err := rawSkip(d); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "object" {
				cur = nil
			}
		}
	}
}

// rawSkip skips to the end of the element just started. Decoder.Skip cannot
// be mixed with RawToken, which does not track the element stack.
func rawSkip(d *xml.Decoder) error {
	for depth := 1; depth > 0; {
		tok, err := d.RawToken()
		if err != nil {
			return err
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return nil
}
