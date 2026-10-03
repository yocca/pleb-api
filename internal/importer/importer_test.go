package importer_test

import (
	"bytes"
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yocca/pleb-api/internal/importer"
	"github.com/yocca/pleb-api/internal/testdb"
)

var westVillage = importer.BBox{South: 40.7280, West: -74.0110, North: 40.7400, East: -73.9990}

func loadFixtures(t *testing.T) ([]importer.Feature, []importer.License) {
	t.Helper()
	osm, err := os.Open("../../testdata/osm_west_village.json")
	if err != nil {
		t.Fatal(err)
	}
	defer osm.Close()
	features, err := importer.ReadOSM(osm)
	if err != nil {
		t.Fatal(err)
	}
	sla, err := os.Open("../../testdata/sla_west_village.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer sla.Close()
	licenses, err := importer.ReadLicenses(sla)
	if err != nil {
		t.Fatal(err)
	}
	return features, licenses
}

func runImport(t *testing.T, pool *pgxpool.Pool, dryRun bool) importer.Report {
	t.Helper()
	features, licenses := loadFixtures(t)
	rep, err := importer.Run(context.Background(), pool, features, licenses,
		importer.Options{BBox: westVillage, DryRun: dryRun, ZIPs: []string{"10014"}})
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

type venueRow struct {
	OSMID, Name, Timezone string
	Serial, Website       *string
	Attribution           []string
}

func venues(t *testing.T, pool *pgxpool.Pool) map[string]venueRow {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT osm_id, name, timezone, sla_serial, website, attribution FROM venues`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]venueRow{}
	for rows.Next() {
		var v venueRow
		if err := rows.Scan(&v.OSMID, &v.Name, &v.Timezone, &v.Serial, &v.Website, &v.Attribution); err != nil {
			t.Fatal(err)
		}
		out[v.OSMID] = v
	}
	return out
}

func TestImportWestVillage(t *testing.T) {
	pool := testdb.New(t)
	rep := runImport(t, pool, false)

	if rep.Created != 5 || rep.Updated != 0 {
		t.Errorf("created %d updated %d, want 5 and 0", rep.Created, rep.Updated)
	}
	// cafe, unnamed bar, out-of-box bar; the relation without coordinates never becomes a feature.
	if got := rep.Skipped; !slices.Equal(got, []string{"node/1004", "node/1005", "node/1006"}) {
		t.Errorf("skipped %v", got)
	}

	vs := venues(t, pool)
	wantIDs := []string{"node/1001", "node/1002", "way/2001", "node/1003", "node/1007"}
	for _, id := range wantIDs {
		v, ok := vs[id]
		if !ok {
			t.Errorf("venue %s missing", id)
			continue
		}
		if v.Timezone != "America/New_York" || !slices.Contains(v.Attribution, importer.OSMAttribution) {
			t.Errorf("%s: timezone %q attribution %v", id, v.Timezone, v.Attribution)
		}
	}
	if len(vs) != len(wantIDs) {
		t.Errorf("got %d venues, want %d", len(vs), len(wantIDs))
	}
	if w := vs["way/2001"].Website; w == nil || *w != "https://hudsonoyster.example" {
		t.Errorf("way/2001 website from contact:website = %v", w)
	}
}

func TestImportMatchesLicenses(t *testing.T) {
	pool := testdb.New(t)
	rep := runImport(t, pool, false)
	vs := venues(t, pool)

	want := map[string]string{"node/1001": "1111111", "node/1002": "2222222", "node/1003": "3333333"}
	for id, serial := range want {
		if got := vs[id].Serial; got == nil || *got != serial {
			t.Errorf("%s serial = %v, want %s", id, got, serial)
		}
	}
	if s := vs["node/1007"].Serial; s != nil {
		t.Errorf("node/1007 has no address but matched %s", *s)
	}
	if rep.Matched != 3 {
		t.Errorf("matched %d, want 3", rep.Matched)
	}
	// Off-premises (5555555) and out-of-area (6666666) licenses are ignored, not reported.
	if len(rep.Unmatched) != 1 || rep.Unmatched[0].Serial != "4444444" {
		t.Errorf("unmatched %+v, want only 4444444", rep.Unmatched)
	}
	if len(vs) != 5 {
		t.Errorf("an unmatched license created a venue: %d venues", len(vs))
	}

	var out bytes.Buffer
	rep.Print(&out, false)
	if !strings.Contains(out.String(), "unmatched license 4444444: MYSTERY BAR LLC (dba MYSTERY LOUNGE), 99 BARROW ST") {
		t.Errorf("report:\n%s", out.String())
	}
}

func TestImportRerunIsSafe(t *testing.T) {
	pool := testdb.New(t)
	runImport(t, pool, false)
	testdb.Exec(t, pool, `
		INSERT INTO happy_hours (venue_id, day_of_week, start_time, end_time, confidence, source_kind)
		SELECT id, 2, '16:00', '19:00', 0.9, 'field_photo' FROM venues WHERE osm_id = 'node/1001'`)

	rep := runImport(t, pool, false)
	if rep.Created != 0 || rep.Updated != 5 {
		t.Errorf("second run created %d updated %d, want 0 and 5", rep.Created, rep.Updated)
	}
	if n := len(venues(t, pool)); n != 5 {
		t.Errorf("%d venues after second run", n)
	}
	var hours int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM happy_hours`).Scan(&hours); err != nil || hours != 1 {
		t.Errorf("happy_hours count %d (err %v), want 1", hours, err)
	}
}

func TestImportDryRun(t *testing.T) {
	pool := testdb.New(t)
	rep := runImport(t, pool, true)
	if rep.Created != 5 || rep.Matched != 3 || len(rep.Unmatched) != 1 {
		t.Errorf("dry run report %+v", rep)
	}
	if n := len(venues(t, pool)); n != 0 {
		t.Fatalf("dry run wrote %d venues", n)
	}

	runImport(t, pool, false)
	if rep := runImport(t, pool, true); rep.Created != 0 || rep.Updated != 5 {
		t.Errorf("dry run after import: created %d updated %d", rep.Created, rep.Updated)
	}
}

func TestImportRejectsAreaOutsideNYC(t *testing.T) {
	pool := testdb.New(t)
	features, licenses := loadFixtures(t)
	boston := importer.BBox{South: 42.35, West: -71.07, North: 42.37, East: -71.05}
	if _, err := importer.Run(context.Background(), pool, features, licenses, importer.Options{BBox: boston}); err == nil {
		t.Fatal("expected an error for a bbox outside NYC")
	}
}

func TestReadLicensesRequiresColumns(t *testing.T) {
	_, err := importer.ReadLicenses(strings.NewReader("Serial,Zip\n1,10014\n"))
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadLicensesCoordinates(t *testing.T) {
	ls, err := importer.ReadLicenses(strings.NewReader(
		"\ufeffSerial Number,Premises Name,Address,Latitude,Longitude\n9,BAR,1 GROVE ST,40.73,-74.00\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) != 1 || ls[0].Lat == nil || *ls[0].Lat != 40.73 || !ls[0].OnPremises() {
		t.Fatalf("got %+v", ls)
	}
}
