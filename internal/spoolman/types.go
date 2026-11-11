package spoolman

// Filament is the part of Spoolman's filament object that print-dis reads.
type Filament struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	Vendor          Vendor `json:"vendor"`
	Material        string `json:"material"`
	ColorHex        string `json:"color_hex"`
	MultiColorHexes string `json:"multi_color_hexes"`
}

// Spool is the part of Spoolman's spool object that print-dis reads.
type Spool struct {
	ID              int      `json:"id"`
	Filament        Filament `json:"filament"`
	RemainingWeight float64  `json:"remaining_weight"`
	Archived        bool     `json:"archived"`
}

// Vendor is a filament maker.
type Vendor struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}
