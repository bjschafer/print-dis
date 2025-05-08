package spoolman

type Filament struct {
	Id                   int            `json:"id"`
	Registered           string         `json:"registered"`
	Name                 string         `json:"name"`
	Vendor               Vendor         `json:"vendor"`
	Material             string         `json:"material"`
	Price                float32        `json:"price"`
	Density              float32        `json:"density"`
	Diameter             float32        `json:"diameter"`
	Weight               float32        `json:"weight"`
	SpoolWeight          float32        `json:"spool_weight"`
	ArticleNumber        string         `json:"article_number"`
	Comment              string         `json:"comment"`
	SettingsExtruderTemp int            `json:"settings_extruder_temp"`
	SettingsBedTemp      int            `json:"settings_bed_temp"`
	ColorHex             string         `json:"color_hex"`
	MultiColorHexes      string         `json:"multi_color_hexes"`
	MultiColorDirection  string         `json:"multi_color_direction"`
	ExternalId           string         `json:"external_id"`
	Extra                map[string]any `json:"extra"`
}

type Spool struct {
	Id              int            `json:"id"`
	Registered      string         `json:"registered"`
	FirstUsed       string         `json:"first_used"` // timestamp
	LastUsed        string         `json:"last_used"`  // timestamp
	Filament        Filament       `json:"filament"`
	Price           float32        `json:"price"`
	RemainingWeight float32        `json:"remaining_weight"`
	InitialWeight   float32        `json:"initial_weight"`
	SpoolWeight     float32        `json:"spool_weight"`
	UsedWeight      float32        `json:"used_weight"`
	RemainingLength float32        `json:"remaining_length"`
	UsedLength      float32        `json:"used_length"`
	Location        string         `json:"location"`
	LotNr           string         `json:"lot_nr"`
	Comment         string         `json:"comment"`
	Archived        bool           `json:"archived"`
	Extra           map[string]any `json:"extra"`
}

type Vendor struct {
	Id               int            `json:"id"`
	Registered       string         `json:"registered"`
	Name             string         `json:"name"`
	Comment          string         `json:"comment"`
	EmptySpoolWeight float32        `json:"empty_spool_weight"`
	ExternalId       string         `json:"external_id"`
	Extra            map[string]any `json:"extra"`
}

type FilamentRequest struct {
	VendorName               string  `json:"vendor.name"`
	VendorId                 int     `json:"vendor.id"`
	Name                     string  `json:"name"`
	Material                 string  `json:"material"`
	ArticleNumber            string  `json:"article_number"`
	ColorHex                 string  `json:"color_hex"`
	ColorSimilarityThreshold float32 `json:"color_similarity_threshold"`
	ExternalId               string  `json:"external_id"`
	CommonRequest
}

type CommonRequest struct {
	Sort   string `json:"sort"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}
