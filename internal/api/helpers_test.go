package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yocca/pleb-api/internal/api"
	"github.com/yocca/pleb-api/internal/testdb"
)

// Origin used by search tests: Grove St & Bedford St, West Village.
const originLat, originLng = 40.7336, -74.0027

var nyc = mustLoadLocation("America/New_York")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

var (
	contractOnce   sync.Once
	contractRouter routers.Router
	contractErr    error
)

func loadContract(t *testing.T) routers.Router {
	t.Helper()
	contractOnce.Do(func() {
		loader := openapi3.NewLoader()
		doc, err := loader.LoadFromFile("../../openapi.yaml")
		if err != nil {
			contractErr = err
			return
		}
		if err = doc.Validate(context.Background()); err != nil {
			contractErr = err
			return
		}
		contractRouter, contractErr = gorillamux.NewRouter(doc)
	})
	if contractErr != nil {
		t.Fatalf("load openapi.yaml: %v", contractErr)
	}
	return contractRouter
}

type testAPI struct {
	t       *testing.T
	pool    *pgxpool.Pool
	handler http.Handler
}

// newAPI starts a server on a fresh database with "now" pinned to now.
func newAPI(t *testing.T, now time.Time) *testAPI {
	t.Helper()
	pool := testdb.New(t)
	return &testAPI{t: t, pool: pool, handler: api.New(pool).WithClock(func() time.Time { return now }).Handler()}
}

// get performs a request, checks the response against openapi.yaml and decodes the body into out.
func (a *testAPI) get(path string, out any) int {
	a.t.Helper()
	return get(a.t, a.handler, path, out)
}

func get(t *testing.T, h http.Handler, path string, out any) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body := rec.Body.Bytes()

	validateResponse(t, req, rec.Code, rec.Header(), body)

	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			t.Fatalf("GET %s: decode %q: %v", path, body, err)
		}
	}
	return rec.Code
}

func validateResponse(t *testing.T, req *http.Request, status int, header http.Header, body []byte) {
	t.Helper()
	router := loadContract(t)
	route, pathParams, err := router.FindRoute(req)
	if err != nil {
		t.Fatalf("GET %s: no route in openapi.yaml: %v", req.URL, err)
	}
	reqInput := &openapi3filter.RequestValidationInput{
		Request: req, PathParams: pathParams, Route: route,
		Options: &openapi3filter.Options{ExcludeRequestBody: true},
	}
	err = openapi3filter.ValidateResponse(context.Background(), &openapi3filter.ResponseValidationInput{
		RequestValidationInput: reqInput,
		Status:                 status,
		Header:                 header,
		Body:                   io.NopCloser(bytes.NewReader(body)),
		Options:                &openapi3filter.Options{IncludeResponseStatus: true, MultiError: true},
	})
	if err != nil {
		t.Errorf("GET %s: response %d does not match openapi.yaml: %v\nbody: %s", req.URL, status, err, body)
	}
}

// venueAt inserts a venue north of the origin by roughly metersNorth.
func (a *testAPI) venueAt(id, name string, metersNorth float64) {
	a.t.Helper()
	lat := originLat + metersNorth/111_195.0
	testdb.Exec(a.t, a.pool, `
		INSERT INTO venues (id, name, address, location, timezone, website, attribution)
		VALUES ($1, $2, '1 Test St, New York, NY 10014',
		        ST_SetSRID(ST_MakePoint($3, $4), 4326)::geography, 'America/New_York',
		        'https://example.com', '{"© OpenStreetMap contributors"}')`,
		id, name, originLng, lat)
}

type hh struct {
	day        int
	start, end string // "" with allDay
	allDay     bool
	confidence float64
	verified   bool
	sourceKind string
	sourceURL  *string
}

func (a *testAPI) happyHour(venueID string, h hh) {
	a.t.Helper()
	if h.sourceKind == "" {
		h.sourceKind = "field_photo"
	}
	var start, end *string
	if h.start != "" {
		start, end = &h.start, &h.end
	}
	testdb.Exec(a.t, a.pool, `
		INSERT INTO happy_hours (venue_id, day_of_week, start_time, end_time, all_day, deals,
		                         confidence, verified, source_kind, source_url)
		VALUES ($1, $2, $3::time, $4::time, $5, '[{"item":"Draft beer","price":5}]', $6, $7, $8, $9)`,
		venueID, h.day, start, end, h.allDay, h.confidence, h.verified, h.sourceKind, h.sourceURL)
}

type venueJSON struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	DistanceM  float64 `json:"distance_m"`
	ActiveNow  bool    `json:"active_now"`
	Website    *string `json:"website"`
	HappyHours []struct {
		DayOfWeek  int     `json:"day_of_week"`
		Start      *string `json:"start"`
		End        *string `json:"end"`
		AllDay     bool    `json:"all_day"`
		Verified   bool    `json:"verified"`
		SourceKind string  `json:"source_kind"`
		SourceURL  *string `json:"source_url"`
	} `json:"happy_hours"`
	Attribution []string `json:"attribution"`
}

type searchJSON struct {
	Venues     []venueJSON `json:"venues"`
	NextCursor *string     `json:"next_cursor"`
}

type errorJSON struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func names(vs []venueJSON) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.Name
	}
	return out
}
