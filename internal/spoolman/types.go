package spoolman

type Filament struct {
	Id                   int
	Registered           string
	Name                 string
	Vendor               Vendor
	Material             string
	Price                float32
	Density              float32
	Diameter             float32
	Weight               float32
	SpoolWeight          float32
	ArticleNumber        string
	Comment              string
	SettingsExtruderTemp int
	SettingsBedTemp      int
	ColorHex             string
	MultiColorHexes      string
	MultiColorDirection  string
	ExternalId           string
	Extra                map[string]any
}

type Spool struct {
	Id              string
	Registered      string
	FirstUsed       string // timestamp
	LastUsed        string // timestamp
	Filament        Filament
	Price           float32
	RemainingWeight float32
	InitialWeight   float32
	SpoolWeight     float32
	UsedWeight      float32
	RemainingLength float32
	UsedLength      float32
	Location        string
	LotNr           string
	Comment         string
	Archived        bool
	Extra           map[string]any
}

type Vendor struct {
	Id               int
	Registered       string
	Name             string
	Comment          string
	EmptySpoolWeight float32
	ExternalId       string
	Extra            map[string]any
}

type FilamentRequest struct {
	VendorName               string `json:"vendor.name"`
	VendorId                 int    `json:"vendor.id"`
	Name                     string
	Material                 string
	ArticleNumber            string
	ColorHex                 string
	ColorSimilarityThreshold float32
	ExternalId               string
	CommonRequest
}

type CommonRequest struct {
	Sort   string
	Limit  int
	Offset int
}
