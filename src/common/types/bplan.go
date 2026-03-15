package types

// BPLAN PIF (Public Interface Format) record types.
// Data is tab-separated; dates are DD-MM-YYYY HH:MM:SS; TLK run times are +MM'SS.

// PIF is the file header record (one per file).
type PIF struct {
	RecordType       string // PIF
	Version          string
	Source           string // e.g. CENTRE
	TOC              string // e.g. NR
	TimetableStart   string
	TimetableEnd     string
	CycleType        string
	CycleIndicator   string
	FileCreationTime  string
	Sequence         string
}

// REF is a reference data record (category codes, descriptions).
type REF struct {
	RecordType   string // REF
	Action       string // e.g. A = Add
	Category     string // e.g. ACC, SER, TOC, TCT, ACT, CAT, PWR
	Subcode      string
	Description  string
}

// LOC is a location (TIPLOC) record.
type LOC struct {
	RecordType  string  // LOC
	Action      string
	Tiploc      string
	Name        string
	StartDate   string
	EndDate     string
	XCoord      string
	YCoord      string
	Optionality string // O / M / T
	Zone        string
	Stanox      string
	StanoxFlag  string // N / Y
	Extra       string // e.g. B, L, P for interchange
}

// TLD is a timing load definition (train/rolling stock class, speed, description).
type TLD struct {
	RecordType  string // TLD
	Action      string
	LoadCode    string // e.g. 08, 150, 313
	LoadSubcode string
	Speed       string // mph
	Reserved    string
	Description string
	TrainType   string // D, DMU, EMU
	SubType     string
	SpeedOrID   string
}

// PLT is a platform record.
type PLT struct {
	RecordType   string // PLT
	Action       string
	Tiploc       string
	PlatformID   string // e.g. 1, 2, DM, DPL
	StartDate    string
	EndDate      string
	PlatformNum  string
	Reserved     string
	Flag         string
	PublicFlag   string // N / Y
}

// NWK is a network link record (from/to locations, link type, direction).
type NWK struct {
	RecordType string // NWK
	Action     string
	FromTiploc string
	ToTiploc   string
	LinkType   string // e.g. BUS, or blank for rail
	Reserved   string
	StartDate  string
	EndDate    string
	Direction  string // D / U
	Direction2 string
	Distance   string
	Flag1      string
	Flag2      string
	Flag3      string
	Zone       string
	Flag4      string
	Reserved2  string
	Extra      string
	Reserved3  string
}

// TLK is a timing link record (run time between two locations for a given load/speed).
type TLK struct {
	RecordType    string // TLK
	Action        string
	FromTiploc    string
	ToTiploc      string
	RouteLinkType string
	TimingLoad    string // or route code
	SubType       string
	Speed         string // mph or class
	LoadVariant   string // e.g. C, H
	Penalty1      string
	Penalty2      string
	StartDate     string
	EndDate       string
	RunTime       string // e.g. +37'00 (minutes'seconds)
	Reserved      string
}

// PIT is the file trailer record (counts per record type).
// Columns repeat in groups of 4: record type label, count, 0, 0.
type PIT struct {
	RecordType string // PIT
	RefLabel   string // REF
	RefCount   string
	RefZero1   string
	RefZero2   string
	TldLabel   string // TLD
	TldCount   string
	TldZero1   string
	TldZero2   string
	LocLabel   string // LOC
	LocCount   string
	LocZero1   string
	LocZero2   string
	PltLabel   string // PLT
	PltCount   string
	PltZero1   string
	PltZero2   string
	NwkLabel   string // NWK
	NwkCount   string
	NwkZero1   string
	NwkZero2   string
	TlkLabel   string // TLK
	TlkCount   string
	TlkZero1   string
	TlkZero2   string
}
