package core

import "fmt"

// PrinterCheck is the eligibility verdict for one printer.
type PrinterCheck struct {
	Printer  Printer
	Eligible bool
	Reasons  []string // why it was excluded; empty when eligible
}

// Check evaluates every printer against the request, in the order given.
func Check(r *Request, printers []Printer) []PrinterCheck {
	out := make([]PrinterCheck, 0, len(printers))
	for _, p := range printers {
		var reasons []string
		if !p.Active {
			reasons = append(reasons, "inactive")
		}
		if r.NeedsMultiMaterial && !p.MultiMaterial {
			reasons = append(reasons, "no multi-material")
		}
		if r.NeedsEnclosure && !p.Enclosed {
			reasons = append(reasons, "not enclosed")
		}
		if r.HasSize() && !fits(*r.SizeX, *r.SizeY, *r.SizeZ, p) {
			reasons = append(reasons, fmt.Sprintf("%d×%d×%d mm does not fit the %d×%d×%d mm bed",
				*r.SizeX, *r.SizeY, *r.SizeZ, p.BedX, p.BedY, p.BedZ))
		}
		out = append(out, PrinterCheck{Printer: p, Eligible: len(reasons) == 0, Reasons: reasons})
	}
	return out
}

// Eligible returns the printers that can take the request.
func Eligible(r *Request, printers []Printer) []Printer {
	var out []Printer
	for _, c := range Check(r, printers) {
		if c.Eligible {
			out = append(out, c.Printer)
		}
	}
	return out
}

// IsEligible reports whether one printer can take the request.
func IsEligible(r *Request, p Printer) bool {
	return Check(r, []Printer{p})[0].Eligible
}

// fits allows rotation about Z (X and Y may swap) but not about any other axis.
func fits(x, y, z int, p Printer) bool {
	if z > p.BedZ {
		return false
	}
	return (x <= p.BedX && y <= p.BedY) || (y <= p.BedX && x <= p.BedY)
}
