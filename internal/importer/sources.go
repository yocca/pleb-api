package importer

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// BBox is a south-west / north-east bounding box in degrees.
type BBox struct{ South, West, North, East float64 }

// ParseBBox parses "south,west,north,east".
func ParseBBox(s string) (BBox, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return BBox{}, errors.New("bbox must be south,west,north,east")
	}
	var v [4]float64
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return BBox{}, fmt.Errorf("bbox: %w", err)
		}
		v[i] = f
	}
	b := BBox{v[0], v[1], v[2], v[3]}
	if b.South >= b.North || b.West >= b.East {
		return BBox{}, errors.New("bbox south/west must be less than north/east")
	}
	return b, nil
}

// Contains reports whether a point lies inside the box.
func (b BBox) Contains(lat, lng float64) bool {
	return lat >= b.South && lat <= b.North && lng >= b.West && lng <= b.East
}

// Within reports whether b lies entirely inside o.
func (b BBox) Within(o BBox) bool {
	return b.South >= o.South && b.North <= o.North && b.West >= o.West && b.East <= o.East
}

// OverpassQuery is the Overpass QL that produces the export the importer reads.
func (b BBox) OverpassQuery() string {
	return fmt.Sprintf(`[out:json][timeout:90];nwr["amenity"~"^(%s)$"](%g,%g,%g,%g);out center tags;`,
		strings.Join(venueAmenities, "|"), b.South, b.West, b.North, b.East)
}

var venueAmenities = []string{"bar", "pub", "biergarten", "restaurant"}

// Feature is one OSM node, way or relation from an Overpass export.
type Feature struct {
	OSMID    string
	Lat, Lng float64
	Tags     map[string]string
}

type overpassExport struct {
	Elements []struct {
		Type   string                      `json:"type"`
		ID     int64                       `json:"id"`
		Lat    *float64                    `json:"lat"`
		Lon    *float64                    `json:"lon"`
		Center *struct{ Lat, Lon float64 } `json:"center"`
		Tags   map[string]string           `json:"tags"`
	} `json:"elements"`
}

// ReadOSM reads an Overpass JSON export ("out center tags"). Ways and relations
// use their center point; elements without coordinates are dropped.
func ReadOSM(r io.Reader) ([]Feature, error) {
	var exp overpassExport
	if err := json.NewDecoder(r).Decode(&exp); err != nil {
		return nil, fmt.Errorf("decode overpass export: %w", err)
	}
	out := make([]Feature, 0, len(exp.Elements))
	for _, e := range exp.Elements {
		f := Feature{OSMID: fmt.Sprintf("%s/%d", e.Type, e.ID), Tags: e.Tags}
		switch {
		case e.Lat != nil && e.Lon != nil:
			f.Lat, f.Lng = *e.Lat, *e.Lon
		case e.Center != nil:
			f.Lat, f.Lng = e.Center.Lat, e.Center.Lon
		default:
			continue
		}
		if f.Tags == nil {
			f.Tags = map[string]string{}
		}
		out = append(out, f)
	}
	return out, nil
}

func (f Feature) isVenue() bool {
	for _, a := range venueAmenities {
		if f.Tags["amenity"] == a {
			return true
		}
	}
	return false
}

// Address builds "123 W 4th St, New York, NY 10014" from addr:* tags, or "" without a street.
func (f Feature) Address() string {
	street := strings.TrimSpace(f.Tags["addr:housenumber"] + " " + f.Tags["addr:street"])
	if f.Tags["addr:street"] == "" {
		return ""
	}
	city := f.Tags["addr:city"]
	if city == "" {
		city = "New York"
	}
	addr := street + ", " + city + ", NY"
	if pc := f.Tags["addr:postcode"]; pc != "" {
		addr += " " + pc
	}
	return addr
}

func (f Feature) firstTag(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(f.Tags[k]); v != "" {
			return v
		}
	}
	return ""
}

// License is one NY State Liquor Authority license record.
type License struct {
	Serial, Type, PremisesName, DBA, Address, ZIP string
	Lat, Lng                                      *float64
}

// Accepted header names for each field; matching is case-insensitive.
var licenseColumns = map[string][]string{
	"serial":  {"license serial number", "serial number", "serial", "license serial no"},
	"type":    {"license type name", "license type", "method of operation", "type"},
	"name":    {"premises name", "premise name", "legal name", "licensee name"},
	"dba":     {"dba", "doing business as (dba)", "doing business as", "trade name"},
	"address": {"actual address of premises (address1)", "address1", "premises address", "address"},
	"zip":     {"zip", "zip code", "zipcode", "postal code"},
	"lat":     {"latitude"},
	"lng":     {"longitude"},
}

// ReadLicenses reads an SLA CSV export. Serial, name and address columns are required.
func ReadLicenses(r io.Reader) ([]License, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read license header: %w", err)
	}
	col := map[string]int{}
	for i, h := range header {
		h = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))
		for field, names := range licenseColumns {
			for _, n := range names {
				if _, seen := col[field]; !seen && h == n {
					col[field] = i
				}
			}
		}
	}
	for _, req := range []string{"serial", "name", "address"} {
		if _, ok := col[req]; !ok {
			return nil, fmt.Errorf("license CSV has no %s column (accepted headers: %s)", req, strings.Join(licenseColumns[req], ", "))
		}
	}

	var out []License
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read license row: %w", err)
		}
		get := func(field string) string {
			if i, ok := col[field]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		l := License{
			Serial: get("serial"), Type: get("type"), PremisesName: get("name"),
			DBA: get("dba"), Address: get("address"), ZIP: get("zip"),
		}
		l.Lat, l.Lng = parseOptFloat(get("lat")), parseOptFloat(get("lng"))
		out = append(out, l)
	}
	return out, nil
}

func parseOptFloat(s string) *float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

// OnPremises reports whether the license allows drinking on site. When the export
// carries no type column every license is kept.
func (l License) OnPremises() bool {
	if l.Type == "" {
		return true
	}
	t := strings.ToLower(l.Type)
	return strings.Contains(t, "on premises") || strings.Contains(t, "on-premises")
}
