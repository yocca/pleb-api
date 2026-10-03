package api_test

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/yocca/pleb-api/internal/testdb"
)

// 2026-10-06 is a Tuesday.
var tuesdayAfternoon = time.Date(2026, 10, 6, 17, 30, 0, 0, nyc)

func searchPath(extra string) string {
	return fmt.Sprintf("/v1/venues?lat=%v&lng=%v%s", originLat, originLng, extra)
}

const (
	v1 = "00000000-0000-0000-0000-000000000001"
	v2 = "00000000-0000-0000-0000-000000000002"
	v3 = "00000000-0000-0000-0000-000000000003"
)

func TestSearchRadiusNearestFirst(t *testing.T) {
	a := newAPI(t, tuesdayAfternoon)
	a.venueAt(v1, "Far", 700)
	a.venueAt(v2, "Near", 100)
	a.venueAt(v3, "Middle", 300)
	for _, id := range []string{v1, v2, v3} {
		a.happyHour(id, hh{day: 2, start: "16:00", end: "19:00", confidence: 0.9})
	}

	var res searchJSON
	if code := a.get(searchPath("&radius_m=500"), &res); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if got := names(res.Venues); !slices.Equal(got, []string{"Near", "Middle"}) {
		t.Fatalf("got %v, want [Near Middle]", got)
	}
	if d := res.Venues[0].DistanceM; d < 95 || d > 105 {
		t.Errorf("Near distance_m = %v, want about 100", d)
	}
	if res.NextCursor != nil {
		t.Errorf("next_cursor = %q on the only page", *res.NextCursor)
	}
}

func TestSearchVisibility(t *testing.T) {
	a := newAPI(t, tuesdayAfternoon)
	a.venueAt(v1, "Low confidence", 100)
	a.happyHour(v1, hh{day: 2, start: "16:00", end: "19:00", confidence: 0.4})
	a.venueAt(v2, "Verified low confidence", 200)
	a.happyHour(v2, hh{day: 2, start: "16:00", end: "19:00", confidence: 0.3, verified: true})
	a.venueAt(v3, "No happy hours", 300)

	var res searchJSON
	a.get(searchPath(""), &res)
	if got := names(res.Venues); !slices.Equal(got, []string{"Verified low confidence"}) {
		t.Fatalf("got %v", got)
	}
	if hh := res.Venues[0].HappyHours; len(hh) != 1 || !hh[0].Verified {
		t.Fatalf("happy_hours = %+v", hh)
	}
}

func TestSearchActiveAt(t *testing.T) {
	a := newAPI(t, tuesdayAfternoon)
	a.venueAt(v1, "Tuesday 4-7", 100)
	a.happyHour(v1, hh{day: 2, start: "16:00", end: "19:00", confidence: 0.9})
	a.venueAt(v2, "Friday late", 200)
	a.happyHour(v2, hh{day: 5, start: "22:00", end: "01:00", confidence: 0.9})
	a.venueAt(v3, "Sunday all day", 300)
	a.happyHour(v3, hh{day: 0, allDay: true, confidence: 0.9})

	cases := []struct {
		name string
		at   time.Time
		want []string
	}{
		{"in progress", tuesdayAfternoon, []string{"Tuesday 4-7"}},
		{"outside every window", time.Date(2026, 10, 6, 20, 0, 0, 0, nyc), []string{}},
		{"start is inclusive", time.Date(2026, 10, 6, 16, 0, 0, 0, nyc), []string{"Tuesday 4-7"}},
		{"end is exclusive", time.Date(2026, 10, 6, 19, 0, 0, 0, nyc), []string{}},
		{"before midnight", time.Date(2026, 10, 9, 23, 0, 0, 0, nyc), []string{"Friday late"}},
		{"crossing midnight into Saturday", time.Date(2026, 10, 10, 0, 30, 0, 0, nyc), []string{"Friday late"}},
		{"after the late window", time.Date(2026, 10, 10, 1, 30, 0, 0, nyc), []string{}},
		{"all day", time.Date(2026, 10, 11, 9, 0, 0, 0, nyc), []string{"Sunday all day"}},
		// Same instant expressed in UTC: evaluation must use the venue's zone, not the request's.
		{"utc input", tuesdayAfternoon.UTC(), []string{"Tuesday 4-7"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var res searchJSON
			a.get(searchPath("&active_at="+url.QueryEscape(tc.at.Format(time.RFC3339))), &res)
			if got := names(res.Venues); !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for _, v := range res.Venues {
				if !v.ActiveNow {
					t.Errorf("%s: active_now = false", v.Name)
				}
			}
		})
	}
}

func TestSearchActiveNowUsesClock(t *testing.T) {
	a := newAPI(t, tuesdayAfternoon)
	a.venueAt(v1, "Open now", 100)
	a.happyHour(v1, hh{day: 2, start: "16:00", end: "19:00", confidence: 0.9})
	a.venueAt(v2, "Later", 200)
	a.happyHour(v2, hh{day: 2, start: "20:00", end: "22:00", confidence: 0.9})

	var res searchJSON
	a.get(searchPath(""), &res)
	got := map[string]bool{}
	for _, v := range res.Venues {
		got[v.Name] = v.ActiveNow
	}
	if !got["Open now"] || got["Later"] || len(got) != 2 {
		t.Fatalf("active_now = %v", got)
	}
}

func TestSearchDayFilter(t *testing.T) {
	a := newAPI(t, tuesdayAfternoon)
	a.venueAt(v1, "Saturday only", 100)
	a.happyHour(v1, hh{day: 6, start: "14:00", end: "17:00", confidence: 0.9})

	var res searchJSON
	a.get(searchPath("&day=1"), &res)
	if len(res.Venues) != 0 {
		t.Fatalf("day=1 returned %v", names(res.Venues))
	}
	a.get(searchPath("&day=6"), &res)
	if got := names(res.Venues); !slices.Equal(got, []string{"Saturday only"}) {
		t.Fatalf("day=6 returned %v", got)
	}
}

func TestSearchPagination(t *testing.T) {
	a := newAPI(t, tuesdayAfternoon)
	// 120 venues; pairs share a location so the id tiebreak is exercised.
	testdb.Exec(t, a.pool, `
		INSERT INTO venues (name, location, timezone)
		SELECT 'Venue ' || i,
		       ST_SetSRID(ST_MakePoint($1, $2 + (i / 2) * 0.0001), 4326)::geography,
		       'America/New_York'
		FROM generate_series(1, 120) AS i`, originLng, originLat)
	testdb.Exec(t, a.pool, `
		INSERT INTO happy_hours (venue_id, day_of_week, start_time, end_time, confidence, source_kind)
		SELECT id, 2, '16:00', '19:00', 0.9, 'website' FROM venues`)

	seen := map[string]bool{}
	var sizes []int
	path := searchPath("&limit=50&radius_m=5000")
	for page := 0; page < 5; page++ {
		var res searchJSON
		if code := a.get(path, &res); code != http.StatusOK {
			t.Fatalf("page %d: status %d", page, code)
		}
		sizes = append(sizes, len(res.Venues))
		for _, v := range res.Venues {
			if seen[v.ID] {
				t.Fatalf("venue %s repeated on page %d", v.Name, page)
			}
			seen[v.ID] = true
		}
		if res.NextCursor == nil {
			break
		}
		path = searchPath("&limit=50&radius_m=5000&cursor=" + url.QueryEscape(*res.NextCursor))
	}
	if !slices.Equal(sizes, []int{50, 50, 20}) || len(seen) != 120 {
		t.Fatalf("page sizes %v, %d distinct venues", sizes, len(seen))
	}
}

func TestSearchProvenance(t *testing.T) {
	a := newAPI(t, tuesdayAfternoon)
	a.venueAt(v1, "Crawled", 100)
	src := "https://example.com/happy-hour"
	a.happyHour(v1, hh{day: 2, start: "16:00", end: "19:00", confidence: 0.9, sourceKind: "website", sourceURL: &src})

	var res searchJSON
	a.get(searchPath(""), &res)
	h := res.Venues[0].HappyHours[0]
	if h.SourceKind != "website" || h.SourceURL == nil || *h.SourceURL != src {
		t.Fatalf("provenance = %s %v", h.SourceKind, h.SourceURL)
	}
	if *h.Start != "16:00" || *h.End != "19:00" {
		t.Fatalf("window = %s-%s", *h.Start, *h.End)
	}
}

func TestVenueDetail(t *testing.T) {
	a := newAPI(t, tuesdayAfternoon)
	a.venueAt(v1, "Detailed", 100)
	a.happyHour(v1, hh{day: 2, start: "16:00", end: "19:00", confidence: 0.9})
	a.happyHour(v1, hh{day: 3, start: "16:00", end: "19:00", confidence: 0.2}) // hidden
	a.venueAt(v2, "No happy hours", 200)

	var v venueJSON
	if code := a.get("/v1/venues/"+v1, &v); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if v.Name != "Detailed" || !v.ActiveNow || len(v.HappyHours) != 1 || v.Website == nil {
		t.Fatalf("got %+v", v)
	}
	if !slices.Contains(v.Attribution, "© OpenStreetMap contributors") {
		t.Fatalf("attribution = %v", v.Attribution)
	}

	var empty venueJSON
	if code := a.get("/v1/venues/"+v2, &empty); code != http.StatusOK || len(empty.HappyHours) != 0 {
		t.Fatalf("venue without happy hours: %d %+v", code, empty)
	}
}

func TestVenueDetailNotFound(t *testing.T) {
	a := newAPI(t, tuesdayAfternoon)
	for _, id := range []string{"00000000-0000-0000-0000-00000000dead", "not-a-uuid"} {
		var body errorJSON
		if code := a.get("/v1/venues/"+id, &body); code != http.StatusNotFound || body.Error.Code != "not_found" {
			t.Errorf("%s: got %d %+v", id, code, body)
		}
	}
}
