package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jack-barr3tt/gbr-engine/src/common/types"
)

var (
	errNotBPLAN     = errors.New("file is not a valid BPLAN dataset")
	errNoPITTrailer = errors.New("no PIT trailer found")
)

type counts struct {
	ref, tld, loc, plt, nwk, tlk int
}

// bplanData holds all parsed records (except PIT) for persistence.
type bplanData struct {
	Pif []types.PIF
	Ref []types.REF
	Loc []types.LOC
	Tld []types.TLD
	Plt []types.PLT
	Nwk []types.NWK
	Tlk []types.TLK
}

// at returns parts[i] if in range, else "".
func at(parts []string, i int) string {
	if i < 0 || i >= len(parts) {
		return ""
	}
	return strings.TrimSpace(parts[i])
}

func parsePIF(parts []string) types.PIF {
	return types.PIF{
		RecordType:       at(parts, 0),
		Version:          at(parts, 1),
		Source:           at(parts, 2),
		TOC:              at(parts, 3),
		TimetableStart:   at(parts, 4),
		TimetableEnd:     at(parts, 5),
		CycleType:        at(parts, 6),
		CycleIndicator:   at(parts, 7),
		FileCreationTime: at(parts, 8),
		Sequence:         at(parts, 9),
	}
}

func parseREF(parts []string) types.REF {
	return types.REF{
		RecordType:  at(parts, 0),
		Action:      at(parts, 1),
		Category:    at(parts, 2),
		Subcode:     at(parts, 3),
		Description: at(parts, 4),
	}
}

func parseLOC(parts []string) types.LOC {
	return types.LOC{
		RecordType:  at(parts, 0),
		Action:      at(parts, 1),
		Tiploc:      at(parts, 2),
		Name:        at(parts, 3),
		StartDate:   at(parts, 4),
		EndDate:     at(parts, 5),
		XCoord:      at(parts, 6),
		YCoord:      at(parts, 7),
		Optionality: at(parts, 8),
		Zone:        at(parts, 9),
		Stanox:      at(parts, 10),
		StanoxFlag:  at(parts, 11),
		Extra:       at(parts, 12),
	}
}

func parseTLD(parts []string) types.TLD {
	return types.TLD{
		RecordType:  at(parts, 0),
		Action:      at(parts, 1),
		LoadCode:    at(parts, 2),
		LoadSubcode: at(parts, 3),
		Speed:       at(parts, 4),
		Reserved:    at(parts, 5),
		Description: at(parts, 6),
		TrainType:   at(parts, 7),
		SubType:     at(parts, 8),
		SpeedOrID:   at(parts, 9),
	}
}

func parsePLT(parts []string) types.PLT {
	return types.PLT{
		RecordType:  at(parts, 0),
		Action:      at(parts, 1),
		Tiploc:      at(parts, 2),
		PlatformID:  at(parts, 3),
		StartDate:   at(parts, 4),
		EndDate:     at(parts, 5),
		PlatformNum: at(parts, 6),
		Reserved:    at(parts, 7),
		Flag:        at(parts, 8),
		PublicFlag:  at(parts, 9),
	}
}

func parseNWK(parts []string) types.NWK {
	return types.NWK{
		RecordType:  at(parts, 0),
		Action:      at(parts, 1),
		FromTiploc:  at(parts, 2),
		ToTiploc:    at(parts, 3),
		LinkType:    at(parts, 4),
		Reserved:    at(parts, 5),
		StartDate:   at(parts, 6),
		EndDate:     at(parts, 7),
		Direction:   at(parts, 8),
		Direction2:  at(parts, 9),
		Distance:    at(parts, 10),
		Flag1:       at(parts, 11),
		Flag2:       at(parts, 12),
		Flag3:       at(parts, 13),
		Zone:        at(parts, 14),
		Flag4:       at(parts, 15),
		Reserved2:   at(parts, 16),
		Extra:       at(parts, 17),
		Reserved3:   at(parts, 18),
	}
}

func parseTLK(parts []string) types.TLK {
	return types.TLK{
		RecordType:    at(parts, 0),
		Action:        at(parts, 1),
		FromTiploc:    at(parts, 2),
		ToTiploc:      at(parts, 3),
		RouteLinkType: at(parts, 4),
		TimingLoad:    at(parts, 5),
		SubType:       at(parts, 6),
		Speed:         at(parts, 7),
		LoadVariant:   at(parts, 8),
		Penalty1:      at(parts, 9),
		Penalty2:      at(parts, 10),
		StartDate:     at(parts, 11),
		EndDate:       at(parts, 12),
		RunTime:       at(parts, 13),
		Reserved:      at(parts, 14),
	}
}

func parsePIT(parts []string) types.PIT {
	return types.PIT{
		RecordType: at(parts, 0),
		RefLabel:   at(parts, 1),
		RefCount:   at(parts, 2),
		RefZero1:   at(parts, 3),
		RefZero2:   at(parts, 4),
		TldLabel:   at(parts, 5),
		TldCount:   at(parts, 6),
		TldZero1:   at(parts, 7),
		TldZero2:   at(parts, 8),
		LocLabel:   at(parts, 9),
		LocCount:   at(parts, 10),
		LocZero1:   at(parts, 11),
		LocZero2:   at(parts, 12),
		PltLabel:   at(parts, 13),
		PltCount:   at(parts, 14),
		PltZero1:   at(parts, 15),
		PltZero2:   at(parts, 16),
		NwkLabel:   at(parts, 17),
		NwkCount:   at(parts, 18),
		NwkZero1:   at(parts, 19),
		NwkZero2:   at(parts, 20),
		TlkLabel:   at(parts, 21),
		TlkCount:   at(parts, 22),
		TlkZero1:   at(parts, 23),
		TlkZero2:   at(parts, 24),
	}
}

// LoadBPLAN reads the BPLAN file at path and returns parsed data, counts, and the PIT record.
func LoadBPLAN(path string) (bplanData, counts, *types.PIT, error) {
	f, err := os.Open(path)
	if err != nil {
		return bplanData{}, counts{}, nil, err
	}
	defer f.Close()

	var data bplanData
	var c counts
	var pit *types.PIT
	firstRecord := true

	scanner := bufio.NewScanner(f)
	const maxLine = 1024 * 1024
	buf := make([]byte, 0, maxLine)
	scanner.Buffer(buf, maxLine)

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		recType := at(parts, 0)

		if firstRecord {
			firstRecord = false
			if recType != "PIF" {
				return bplanData{}, counts{}, nil, fmt.Errorf("%w: first record must be PIF, got %q", errNotBPLAN, recType)
			}
		}

		switch recType {
		case "PIF":
			data.Pif = append(data.Pif, parsePIF(parts))
		case "REF":
			c.ref++
			data.Ref = append(data.Ref, parseREF(parts))
		case "LOC":
			c.loc++
			data.Loc = append(data.Loc, parseLOC(parts))
		case "TLD":
			c.tld++
			data.Tld = append(data.Tld, parseTLD(parts))
		case "PLT":
			c.plt++
			data.Plt = append(data.Plt, parsePLT(parts))
		case "NWK":
			c.nwk++
			data.Nwk = append(data.Nwk, parseNWK(parts))
		case "TLK":
			c.tlk++
			data.Tlk = append(data.Tlk, parseTLK(parts))
		case "PIT":
			p := parsePIT(parts)
			pit = &p
		}
	}

	if err := scanner.Err(); err != nil {
		return bplanData{}, counts{}, nil, err
	}

	if pit == nil {
		return bplanData{}, counts{}, nil, fmt.Errorf("%w", errNoPITTrailer)
	}

	return data, c, pit, nil
}

// ValidatePIT checks that parsed counts match the PIT trailer record.
func ValidatePIT(c counts, pit *types.PIT) (ok bool, details []string) {
	if pit == nil {
		return false, []string{"no PIT record found"}
	}

	refWant, _ := strconv.Atoi(pit.RefCount)
	tldWant, _ := strconv.Atoi(pit.TldCount)
	locWant, _ := strconv.Atoi(pit.LocCount)
	pltWant, _ := strconv.Atoi(pit.PltCount)
	nwkWant, _ := strconv.Atoi(pit.NwkCount)
	tlkWant, _ := strconv.Atoi(pit.TlkCount)

	ok = true
	if c.ref != refWant {
		ok = false
		details = append(details, "REF: got "+strconv.Itoa(c.ref)+", PIT says "+pit.RefCount)
	}
	if c.tld != tldWant {
		ok = false
		details = append(details, "TLD: got "+strconv.Itoa(c.tld)+", PIT says "+pit.TldCount)
	}
	if c.loc != locWant {
		ok = false
		details = append(details, "LOC: got "+strconv.Itoa(c.loc)+", PIT says "+pit.LocCount)
	}
	if c.plt != pltWant {
		ok = false
		details = append(details, "PLT: got "+strconv.Itoa(c.plt)+", PIT says "+pit.PltCount)
	}
	if c.nwk != nwkWant {
		ok = false
		details = append(details, "NWK: got "+strconv.Itoa(c.nwk)+", PIT says "+pit.NwkCount)
	}
	if c.tlk != tlkWant {
		ok = false
		details = append(details, "TLK: got "+strconv.Itoa(c.tlk)+", PIT says "+pit.TlkCount)
	}
	return ok, details
}
