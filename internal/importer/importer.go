// Package importer builds the venue list for a launch area from an OpenStreetMap
// export and cross-references NY State Liquor Authority on-premises licenses.
package importer

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yocca/pleb-api/internal/store"
)

// OSMAttribution is the credit ODbL requires on venues sourced from OpenStreetMap.
const OSMAttribution = "© OpenStreetMap contributors"

// NYC bounds the areas the importer accepts while every venue is stored as America/New_York.
var NYC = BBox{South: 40.47, West: -74.27, North: 40.93, East: -73.68}

const nycTimezone = "America/New_York"

// Minimum trigram similarity between an OSM name and a license's premises name or DBA.
const nameMatchThreshold = 0.4

// Options controls one import run.
type Options struct {
	BBox   BBox
	DryRun bool
	// ZIPs limits licenses to these ZIP codes when the export has no coordinates.
	ZIPs []string
}

// Report summarizes what a run did (or, in a dry run, would do).
type Report struct {
	Created, Updated int
	Skipped          []string // OSM ids of unnamed or out-of-area features
	Matched          int
	Unmatched        []License
}

// Print writes the report in the format the CLI shows.
func (r Report) Print(w io.Writer, dryRun bool) {
	verb := ""
	if dryRun {
		verb = " (dry run, nothing written)"
	}
	fmt.Fprintf(w, "venues: %d created, %d updated, %d skipped%s\n", r.Created, r.Updated, len(r.Skipped), verb)
	fmt.Fprintf(w, "licenses: %d matched, %d unmatched\n", r.Matched, len(r.Unmatched))
	for _, l := range r.Unmatched {
		name := l.PremisesName
		if l.DBA != "" {
			name += " (dba " + l.DBA + ")"
		}
		fmt.Fprintf(w, "  unmatched license %s: %s, %s\n", l.Serial, name, l.Address)
	}
}

type candidate struct {
	Feature
	name, address, normAddr string
	serial                  *string
}

// Run imports features and licenses for opts.BBox.
func Run(ctx context.Context, pool *pgxpool.Pool, features []Feature, licenses []License, opts Options) (Report, error) {
	var rep Report
	if !opts.BBox.Within(NYC) {
		return rep, fmt.Errorf("bbox is outside New York City; only NYC imports are supported until venue timezones are looked up")
	}

	var cands []*candidate
	for _, f := range features {
		name := f.firstTag("name")
		if !f.isVenue() || !opts.BBox.Contains(f.Lat, f.Lng) || name == "" {
			rep.Skipped = append(rep.Skipped, f.OSMID)
			continue
		}
		addr := f.Address()
		cands = append(cands, &candidate{Feature: f, name: name, address: addr, normAddr: NormalizeAddress(addr)})
	}

	for _, l := range licenses {
		if !l.OnPremises() || !licenseInArea(l, opts) {
			continue
		}
		if c := bestMatch(cands, l); c != nil {
			serial := l.Serial
			c.serial = &serial
			rep.Matched++
		} else {
			rep.Unmatched = append(rep.Unmatched, l)
		}
	}

	q := store.New(pool)
	osmIDs := make([]string, len(cands))
	for i, c := range cands {
		osmIDs[i] = c.OSMID
	}
	existing, err := q.VenueKeysByOSMIDs(ctx, osmIDs)
	if err != nil {
		return rep, fmt.Errorf("load existing venues: %w", err)
	}
	if opts.DryRun {
		rep.Updated = len(existing)
		rep.Created = len(cands) - len(existing)
		return rep, nil
	}

	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		qtx := q.WithTx(tx)
		for _, c := range cands {
			row, err := qtx.UpsertOSMVenue(ctx, store.UpsertOSMVenueParams{
				OsmID:       &c.OSMID,
				SlaSerial:   c.serial,
				Name:        c.name,
				Address:     nilIfEmpty(c.address),
				Lat:         c.Lat,
				Lng:         c.Lng,
				Timezone:    nycTimezone,
				Website:     nilIfEmpty(c.firstTag("website", "contact:website")),
				Phone:       nilIfEmpty(c.firstTag("phone", "contact:phone")),
				Attribution: []string{OSMAttribution},
			})
			if err != nil {
				return fmt.Errorf("upsert %s: %w", c.OSMID, err)
			}
			if row.Inserted {
				rep.Created++
			} else {
				rep.Updated++
			}
		}
		return nil
	})
	return rep, err
}

func licenseInArea(l License, opts Options) bool {
	if l.Lat != nil && l.Lng != nil {
		return opts.BBox.Contains(*l.Lat, *l.Lng)
	}
	if len(opts.ZIPs) > 0 {
		zip := l.ZIP
		if len(zip) > 5 {
			zip = zip[:5]
		}
		return slices.Contains(opts.ZIPs, zip)
	}
	return true
}

// bestMatch returns the candidate at the license's address whose name is most similar
// to the license's premises name or DBA, if that similarity clears the threshold.
func bestMatch(cands []*candidate, l License) *candidate {
	addr := NormalizeAddress(l.Address)
	if addr == "" {
		return nil
	}
	var best *candidate
	bestScore := nameMatchThreshold
	for _, c := range cands {
		if c.normAddr != addr || c.serial != nil {
			continue
		}
		score := max(Similarity(c.name, l.PremisesName), Similarity(c.name, l.DBA))
		if score >= bestScore {
			best, bestScore = c, score
		}
	}
	return best
}

func nilIfEmpty(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}
