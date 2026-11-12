package web

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/spoolman"
)

// The color picker posts, for each color slot with key K (0, 1, ...):
//
//	color_K       ""         any color
//	              "c:blue"   a rough color from core.RoughColors
//	              "hex"      the exact color in color_hex_K
//	              "f:12"     Spoolman filament 12
//	color_hex_K   "#rrggbb"  from <input type=color>; always posted
//	color_note_K  free text
//
// plus colors_form=1 so a form without the picker leaves colors alone.
// Slot keys need not be contiguous: script adds and removes slots.

// maxSlotKey bounds the slot keys read from a form.
const maxSlotKey = 64

// defaultExactHex is the starting value of the exact-color input.
const defaultExactHex = "#808080"

// colorSlot is one color on the form.
type colorSlot struct {
	Key       string // the form field suffix
	Num       int    // 1-based, for the legend
	Choice    core.ColorChoice
	Value     string // the checked radio's value
	ExactHex  string // the exact-color input's value
	Kept      bool   // a filament pick the list does not have; shown as its own tile
	KeptGone  bool   // ...because Spoolman no longer has it (not because it is down)
	ShowLabel bool   // more than one slot, so number the legends
}

// shelfMatch is what the owner has on the shelf near one requested color.
type shelfMatch struct {
	Choice  core.ColorChoice
	Options []spoolman.FilamentOption
}

func slotValue(c core.ColorChoice) string {
	switch {
	case c.FilamentID != nil:
		return "f:" + strconv.Itoa(*c.FilamentID)
	case c.Rough != "":
		return "c:" + c.Rough
	case len(c.Hexes) > 0:
		return "hex"
	}
	return ""
}

// buildSlots lays out the request's colors, plus extra blank ones, for the form.
func buildSlots(cs []core.ColorChoice, extra int, opts []spoolman.FilamentOption, down bool) []colorSlot {
	known := map[int]bool{}
	for _, o := range opts {
		known[o.ID] = true
	}
	n := max(len(cs)+extra, 1)
	slots := make([]colorSlot, n)
	for i := range slots {
		s := colorSlot{Key: strconv.Itoa(i), Num: i + 1, ExactHex: defaultExactHex, ShowLabel: n > 1}
		if i < len(cs) {
			c := cs[i]
			s.Choice, s.Value = c, slotValue(c)
			if s.Value == "hex" {
				s.ExactHex = c.Hex()
			}
			if c.FilamentID != nil && !known[*c.FilamentID] {
				s.Kept, s.KeptGone = true, !down
			}
		}
		slots[i] = s
	}
	return slots
}

// formMaterials is the material chips: the common ones and Spoolman's.
func formMaterials(opts []spoolman.FilamentOption) []string {
	ms := slices.Clone(core.CommonMaterials)
	for _, m := range spoolman.Materials(opts) {
		if !slices.Contains(ms, m) {
			ms = append(ms, m)
		}
	}
	return ms
}

// applyColors reads the color slots into req.Colors. A filament pick is
// looked up in the source and snapshotted; what the client might post for
// its label or color is never read. A pick the source no longer has is
// kept only when the request already had it. When the request names no
// material, the picked filaments' material is used.
func (s *Server) applyColors(r *http.Request, req *core.Request, errs *formErrors) {
	if r.PostFormValue("colors_form") == "" {
		return
	}
	var opts []spoolman.FilamentOption
	loaded := false
	filaments := func() []spoolman.FilamentOption {
		if !loaded && s.Filaments != nil {
			loaded = true
			var err error
			if opts, err = s.Filaments.Filaments(r.Context()); err != nil {
				s.Log.Warn("filament list unavailable", "err", err)
			}
		}
		return opts
	}

	var out []core.ColorChoice
	for i := range maxSlotKey {
		k := strconv.Itoa(i)
		if !r.PostForm.Has("color_"+k) && !r.PostForm.Has("color_hex_"+k) && !r.PostForm.Has("color_note_"+k) {
			continue
		}
		c := core.ColorChoice{Note: trimmed(r, "color_note_"+k)}
		if len([]rune(c.Note)) > 200 {
			errs.add("A color note is too long (at most 200 characters).")
			continue
		}
		v := trimmed(r, "color_"+k)
		switch {
		case v == "":
			if c.Note == "" {
				continue // a blank slot
			}
		case v == "hex":
			h := normHex(trimmed(r, "color_hex_"+k))
			if h == "" {
				errs.add("Pick the exact color again; that one could not be read.")
				continue
			}
			c.Hexes = []string{h}
		case strings.HasPrefix(v, "c:"):
			rc, ok := core.RoughColorByKey(strings.TrimPrefix(v, "c:"))
			if !ok {
				errs.add("Pick a color from the list.")
				continue
			}
			c.Rough, c.Label, c.Hexes = rc.Key, rc.Name, []string{rc.Hex}
		case strings.HasPrefix(v, "f:"):
			id, ok := optID(strings.TrimPrefix(v, "f:"))
			if !ok {
				errs.add("Pick a filament from the list, or choose Any.")
				continue
			}
			if !s.snapshotFilament(int(id), &c, filaments(), req.Colors) {
				errs.add("That filament is not in the list any more. Pick another, or choose Any.")
				continue
			}
		default:
			errs.add("Pick a color from the list, or choose Any.")
			continue
		}
		out = append(out, c)
	}
	if len(out) > core.MaxColors {
		errs.add("A request can list at most %d colors.", core.MaxColors)
	}
	req.Colors = out
	if req.Material == "" {
		req.Material = core.ColorsMaterial(out)
	}
}

// snapshotFilament fills c from filament id: from the list when it is
// there, else from the request's existing choice of it.
func (s *Server) snapshotFilament(id int, c *core.ColorChoice, opts []spoolman.FilamentOption, before []core.ColorChoice) bool {
	for _, o := range opts {
		if o.ID == id {
			c.FilamentID, c.Label, c.Material, c.Hexes = &id, o.Label, o.Material, slices.Clone(o.Hexes)
			return true
		}
	}
	for _, b := range before {
		if b.FilamentID != nil && *b.FilamentID == id {
			c.FilamentID, c.Label, c.Material, c.Hexes = &id, b.Label, b.Material, slices.Clone(b.Hexes)
			return true
		}
	}
	return false
}

// shelfMatches finds in-stock filaments near each color that was asked for
// roughly (a palette or exact color rather than a picked filament).
func shelfMatches(req *core.Request, opts []spoolman.FilamentOption) []shelfMatch {
	var out []shelfMatch
	for _, c := range req.Colors {
		if c.FilamentID != nil || c.Hex() == "" {
			continue
		}
		out = append(out, shelfMatch{Choice: c, Options: spoolman.Closest(opts, c.Hex(), req.Material, 4)})
	}
	return out
}
