package db_test

import (
	"context"
	"testing"

	"github.com/yocca/pleb-api/internal/db"
	"github.com/yocca/pleb-api/internal/testdb"
)

func TestMigrationsUpDownUp(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.NewUnmigrated(t)

	for _, step := range []string{"up", "reset", "up"} {
		if err := db.Migrate(ctx, pool, step); err != nil {
			t.Fatalf("migrate %s: %v", step, err)
		}
	}

	var tables int
	err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name IN ('venues', 'submissions', 'extractions', 'happy_hours', 'visible_happy_hours')`).Scan(&tables)
	if err != nil {
		t.Fatal(err)
	}
	if tables != 5 {
		t.Fatalf("got %d of 5 tables and views after up/reset/up", tables)
	}
}

func TestVenueRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)

	testdb.Exec(t, pool, `
		INSERT INTO venues (name, location, timezone)
		VALUES ('Smoke Test Bar', ST_SetSRID(ST_MakePoint(-74.0027, 40.7336), 4326)::geography, 'America/New_York')`)

	var name string
	var lat float64
	err := pool.QueryRow(ctx, `SELECT name, ST_Y(location::geometry) FROM venues`).Scan(&name, &lat)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Smoke Test Bar" || lat != 40.7336 {
		t.Fatalf("got %q at lat %v", name, lat)
	}
}

func TestHappyHourConstraints(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	testdb.Exec(t, pool, `
		INSERT INTO venues (id, name, location, timezone)
		VALUES ('00000000-0000-0000-0000-000000000001', 'Bar', ST_MakePoint(-74, 40.7)::geography, 'America/New_York')`)

	bad := []string{
		// a timed window needs both ends
		`INSERT INTO happy_hours (venue_id, day_of_week, start_time, confidence, source_kind)
		 VALUES ('00000000-0000-0000-0000-000000000001', 1, '16:00', 0.9, 'website')`,
		// weekday out of range
		`INSERT INTO happy_hours (venue_id, day_of_week, all_day, confidence, source_kind)
		 VALUES ('00000000-0000-0000-0000-000000000001', 7, true, 0.9, 'website')`,
		// unknown source
		`INSERT INTO happy_hours (venue_id, day_of_week, all_day, confidence, source_kind)
		 VALUES ('00000000-0000-0000-0000-000000000001', 1, true, 0.9, 'google')`,
	}
	for _, sql := range bad {
		if _, err := pool.Exec(ctx, sql); err == nil {
			t.Errorf("expected constraint violation for: %s", sql)
		}
	}
}
