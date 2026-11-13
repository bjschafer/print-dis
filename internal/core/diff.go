package core

import (
	"slices"
	"time"
)

func eqPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func eqDate(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Format(time.DateOnly) == b.Format(time.DateOnly)
}

// ChangedFields names the user-visible fields that differ between two
// versions of a request, for the body of an edit event.
func ChangedFields(a, b *Request) []string {
	var out []string
	add := func(changed bool, name string) {
		if changed {
			out = append(out, name)
		}
	}
	add(a.RequesterID != b.RequesterID, "requester")
	add(a.Title != b.Title, "title")
	add(a.Quantity != b.Quantity, "quantity")
	add(a.Printed != b.Printed, "printed")
	add(!eqDate(a.NeededBy, b.NeededBy), "needed by")
	add(a.Timing != b.Timing, "timing")
	add(a.Notes != b.Notes, "notes")
	add(!eqPtr(a.SourceURL, b.SourceURL) || !eqPtr(a.SourceTitle, b.SourceTitle), "link")
	add(a.Material != b.Material, "material")
	add(!slices.EqualFunc(a.Colors, b.Colors, ColorChoice.Equal), "color")
	add(a.NeedsMultiMaterial != b.NeedsMultiMaterial, "multi-material")
	add(a.NeedsEnclosure != b.NeedsEnclosure, "enclosure")
	add(!eqPtr(a.SizeX, b.SizeX) || !eqPtr(a.SizeY, b.SizeY) || !eqPtr(a.SizeZ, b.SizeZ), "size")
	add(!eqPtr(a.PrinterID, b.PrinterID), "printer")
	add(a.Project != b.Project, "project")
	return slices.Clip(out)
}
