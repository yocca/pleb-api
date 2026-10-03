package importer_test

import (
	"context"
	"math"
	"testing"

	"github.com/yocca/pleb-api/internal/importer"
	"github.com/yocca/pleb-api/internal/testdb"
)

func TestNormalizeAddress(t *testing.T) {
	same := [][2]string{
		{"75 Seventh Avenue South, New York, NY 10014", "75 7TH AVE S"},
		{"12 Bedford Street", "12 BEDFORD ST"},
		{"123 West 4th Street, Apt 2", "123 W 4 ST"},
		{"1 Avenue of the Americas", "1 6th Ave"},
		{"510 Hudson St. #3", "510 hudson street"},
		{"45 Grove St Suite 200", "45 Grove Street"},
	}
	for _, p := range same {
		if a, b := importer.NormalizeAddress(p[0]), importer.NormalizeAddress(p[1]); a != b {
			t.Errorf("%q -> %q but %q -> %q", p[0], a, p[1], b)
		}
	}
	if importer.NormalizeAddress("75 Grove St") == importer.NormalizeAddress("77 Grove St") {
		t.Error("different house numbers normalized equal")
	}
}

// Similarity must agree with pg_trgm's similarity(), which the design is based on.
func TestSimilarityMatchesPgTrgm(t *testing.T) {
	pool := testdb.New(t)
	testdb.Exec(t, pool, `CREATE EXTENSION IF NOT EXISTS pg_trgm`)

	pairs := [][2]string{
		{"Grove Street Tap", "GROVE STREET TAP"},
		{"Seventh Avenue Diner", "SEVENTH AVE DINER CORP"},
		{"The Bedford Arms", "BEDFORD ARMS INC"},
		{"Hudson Oyster Room", "MYSTERY LOUNGE"},
		{"Café Olé!", "cafe ole"},
		{"a", "ab"},
		{"", "anything"},
	}
	for _, p := range pairs {
		var want float64
		if err := pool.QueryRow(context.Background(), `SELECT similarity($1, $2)::float8`, p[0], p[1]).Scan(&want); err != nil {
			t.Fatal(err)
		}
		if got := importer.Similarity(p[0], p[1]); math.Abs(got-want) > 1e-6 {
			t.Errorf("Similarity(%q, %q) = %.4f, pg_trgm says %.4f", p[0], p[1], got, want)
		}
	}
}
