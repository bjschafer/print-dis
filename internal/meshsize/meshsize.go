// Package meshsize measures the bounding box of a model file (STL, OBJ or
// 3MF), so a request can be checked against printer beds without anyone
// typing its size. Units are assumed to be millimetres where the format does
// not say; 3MF's unit attribute is honoured.
package meshsize

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// Box is an axis-aligned bounding box in millimetres.
type Box struct {
	Min, Max [3]float64
	n        int // points added
}

func (b *Box) add(x, y, z float64) {
	if !finite(x) || !finite(y) || !finite(z) {
		return
	}
	p := [3]float64{x, y, z}
	if b.n == 0 {
		b.Min, b.Max = p, p
	} else {
		for i := range 3 {
			b.Min[i] = min(b.Min[i], p[i])
			b.Max[i] = max(b.Max[i], p[i])
		}
	}
	b.n++
}

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

// Empty reports whether no points were seen.
func (b Box) Empty() bool { return b.n == 0 }

// Size returns the box's extent, each axis rounded up to a whole millimetre
// (ignoring float noise of a hundredth) and at least 1.
func (b Box) Size() (x, y, z int) {
	r := func(i int) int {
		return max(1, int(math.Ceil(b.Max[i]-b.Min[i]-0.01)))
	}
	return r(0), r(1), r(2)
}

// maxExtent rejects results that are clearly not millimetres of a real part.
const maxExtent = 100_000

// ErrUnsupported means the format is not one this package reads.
var ErrUnsupported = errors.New("meshsize: unsupported format")

// ErrNoGeometry means the file parsed but held no vertices.
var ErrNoGeometry = errors.New("meshsize: no geometry")

// Supported reports whether ext (with the dot, lower case) can be measured.
func Supported(ext string) bool {
	switch ext {
	case ".stl", ".obj", ".3mf":
		return true
	}
	return false
}

// Measure reads a whole model file of the given extension and size and
// returns its bounding box. A 3MF is read fully into memory, so callers
// should bound size.
func Measure(ext string, r io.Reader, size int64) (Box, error) {
	var b Box
	var err error
	switch ext {
	case ".stl":
		b, err = stl(r, size)
	case ".obj":
		b, err = obj(r)
	case ".3mf":
		b, err = threeMF(r, size)
	default:
		return Box{}, ErrUnsupported
	}
	if err != nil {
		return Box{}, err
	}
	if b.Empty() {
		return Box{}, ErrNoGeometry
	}
	for i := range 3 {
		if b.Max[i]-b.Min[i] > maxExtent {
			return Box{}, fmt.Errorf("meshsize: implausible extent %.0f mm", b.Max[i]-b.Min[i])
		}
	}
	return b, nil
}

// stl reads binary or ASCII STL. A file is binary when its length matches
// the triangle count in its header, whatever its first bytes say (many
// binary files start with "solid" too).
func stl(r io.Reader, size int64) (Box, error) {
	head := make([]byte, 84)
	n, err := io.ReadFull(r, head)
	head = head[:n]
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return Box{}, err
	}
	if n == 84 {
		count := int64(binary.LittleEndian.Uint32(head[80:]))
		if 84+50*count == size {
			return binarySTL(r, count)
		}
	}
	return asciiVertices(io.MultiReader(bytes.NewReader(head), r), "vertex")
}

func binarySTL(r io.Reader, count int64) (Box, error) {
	var b Box
	br := bufio.NewReaderSize(r, 1<<16)
	var tri [50]byte
	f := func(off int) float64 {
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(tri[off:])))
	}
	for range count {
		if _, err := io.ReadFull(br, tri[:]); err != nil {
			return Box{}, fmt.Errorf("meshsize: truncated STL: %w", err)
		}
		for v := range 3 {
			off := 12 + 12*v
			b.add(f(off), f(off+4), f(off+8))
		}
	}
	return b, nil
}

// asciiVertices reads lines of the form "<keyword> x y z" (ASCII STL's
// "vertex", OBJ's "v").
func asciiVertices(r io.Reader, keyword string) (Box, error) {
	var b Box
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 || fields[0] != keyword {
			continue
		}
		x, ex := strconv.ParseFloat(fields[1], 64)
		y, ey := strconv.ParseFloat(fields[2], 64)
		z, ez := strconv.ParseFloat(fields[3], 64)
		if ex != nil || ey != nil || ez != nil {
			continue
		}
		b.add(x, y, z)
	}
	return b, sc.Err()
}

func obj(r io.Reader) (Box, error) { return asciiVertices(r, "v") }
