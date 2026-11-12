package core

import (
	"slices"
	"strings"
)

// MaxColors is how many colors one request can list.
const MaxColors = 8

// ColorChoice is one color a request asks for. A request with no choices
// takes any color. A choice is one of: a filament picked from Spoolman
// (FilamentID set), a rough color from the palette (Rough set), an exact
// color (Hexes set, nothing else), or no color at all with only a Note.
// Label, Material and Hexes are snapshots taken when the choice was made,
// so a choice still reads correctly after its filament leaves Spoolman.
type ColorChoice struct {
	FilamentID *int     `json:"filament_id,omitempty"`
	Rough      string   `json:"rough,omitempty"`    // a RoughColors key
	Label      string   `json:"label,omitempty"`    // "Elegoo Blue", or the palette name
	Material   string   `json:"material,omitempty"` // the picked filament's material
	Hexes      []string `json:"hexes,omitempty"`    // "#rrggbb"; several for a multi-color filament
	Note       string   `json:"note,omitempty"`     // free text: "matte", "glow in the dark"
}

// Name is what the choice is called on a card or in a list.
func (c ColorChoice) Name() string {
	switch {
	case c.Label != "":
		return c.Label
	case len(c.Hexes) > 0:
		return "Exact color"
	}
	return "Any color"
}

// IsAny reports whether the choice names no color.
func (c ColorChoice) IsAny() bool { return c.FilamentID == nil && c.Rough == "" && len(c.Hexes) == 0 }

// Equal reports whether two choices are the same.
func (c ColorChoice) Equal(o ColorChoice) bool {
	return eqPtr(c.FilamentID, o.FilamentID) && c.Rough == o.Rough && c.Label == o.Label &&
		c.Material == o.Material && slices.Equal(c.Hexes, o.Hexes) && c.Note == o.Note
}

// Hex is the choice's main color, or "" when it has none.
func (c ColorChoice) Hex() string {
	if len(c.Hexes) == 0 {
		return ""
	}
	return c.Hexes[0]
}

// RoughColor is a palette entry for "roughly this color".
type RoughColor struct {
	Key  string
	Name string
	Hex  string
}

// RoughColors is the palette offered when someone has a rough idea of a
// color but no particular filament in mind.
var RoughColors = []RoughColor{
	{"red", "Red", "#d32f2f"},
	{"orange", "Orange", "#f57c00"},
	{"yellow", "Yellow", "#fbc02d"},
	{"green", "Green", "#388e3c"},
	{"teal", "Teal", "#00897b"},
	{"blue", "Blue", "#1e66d0"},
	{"purple", "Purple", "#7b1fa2"},
	{"pink", "Pink", "#ec6fa8"},
	{"brown", "Brown", "#795548"},
	{"black", "Black", "#161616"},
	{"grey", "Gray", "#8a8f98"},
	{"white", "White", "#f4f4f2"},
	{"silver", "Silver", "#c0c4ca"},
	{"gold", "Gold", "#c9a227"},
	{"clear", "Clear", "#dcebf0"},
}

// RoughColorByKey returns the palette entry with key k.
func RoughColorByKey(k string) (RoughColor, bool) {
	for _, c := range RoughColors {
		if c.Key == k {
			return c, true
		}
	}
	return RoughColor{}, false
}

// roughAliases are other words for palette entries.
var roughAliases = map[string]string{
	"gray": "grey", "transparent": "clear", "natural": "clear", "violet": "purple", "navy": "blue",
}

// ParseColorNote turns a free-text color, such as one from the old sheet,
// into choices: a palette color when the text is just its name ("Red",
// "gray"), otherwise a note. Empty text means any color.
func ParseColorNote(s string) []ColorChoice {
	s = strings.TrimSpace(s)
	k := strings.ToLower(s)
	if k == "" || k == "any" {
		return nil
	}
	if a, ok := roughAliases[k]; ok {
		k = a
	}
	if rc, ok := RoughColorByKey(k); ok {
		return []ColorChoice{{Rough: rc.Key, Label: rc.Name, Hexes: []string{rc.Hex}}}
	}
	return []ColorChoice{{Note: s}}
}

// ColorsMaterial is the material implied by picked filaments when the
// request does not name one: their material when they agree, all of them
// joined with " + " when they do not, or "" when none was picked.
func ColorsMaterial(cs []ColorChoice) string {
	var ms []string
	for _, c := range cs {
		if m := strings.ToUpper(strings.TrimSpace(c.Material)); m != "" && !slices.Contains(ms, m) {
			ms = append(ms, m)
		}
	}
	return strings.Join(ms, " + ")
}

// CommonMaterials are offered on the form even when Spoolman has none.
var CommonMaterials = []string{"PLA", "PETG", "ABS", "ASA", "TPU"}
