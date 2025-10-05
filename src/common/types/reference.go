package types

type StationReference struct {
	Version     string         `json:"version"`
	StationList []StationEntry `json:"StationList"`
}

type StationEntry struct {
	Crs   string `json:"crs"`
	Value string `json:"Value"`
}

type TOCReference struct {
	Version string     `json:"version"`
	TOCList []TOCEntry `json:"TOCList"`
}

type TOCEntry struct {
	TOC   string `json:"toc"`
	Value string `json:"Value"`
}
