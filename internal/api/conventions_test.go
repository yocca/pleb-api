package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yocca/pleb-api/internal/api"
)

func TestHealthUp(t *testing.T) {
	a := newAPI(t, time.Now())
	var body map[string]string
	if code := a.get("/healthz", &body); code != http.StatusOK || body["status"] != "ok" {
		t.Fatalf("got %d %v, want 200 ok", code, body)
	}
}

func TestHealthDatabaseDown(t *testing.T) {
	// Nothing listens on port 1, so every connection attempt fails.
	pool, err := pgxpool.New(context.Background(), "postgres://pleb:pleb@127.0.0.1:1/pleb?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	h := api.New(pool).Handler()

	var body errorJSON
	if code := get(t, h, "/healthz", &body); code != http.StatusServiceUnavailable || body.Error.Code != "unavailable" {
		t.Fatalf("got %d %+v, want 503 unavailable", code, body)
	}
}

func TestDatabaseFailureIsGeneric500(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://pleb:pleb@127.0.0.1:1/pleb?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	h := api.New(pool).Handler()

	for _, path := range []string{
		"/v1/venues?lat=40.7&lng=-74",
		"/v1/venues/00000000-0000-0000-0000-000000000001",
	} {
		var body errorJSON
		code := get(t, h, path, &body)
		if code != http.StatusInternalServerError || body.Error.Code != "internal" {
			t.Errorf("%s: got %d %+v, want 500 internal", path, code, body)
		}
		if msg := strings.ToLower(body.Error.Message); strings.Contains(msg, "dial") || strings.Contains(msg, "127.0.0.1") || strings.Contains(msg, "sql") {
			t.Errorf("%s: 500 leaks internal detail: %q", path, body.Error.Message)
		}
	}
}

func TestInvalidParams(t *testing.T) {
	a := newAPI(t, time.Now())
	cases := map[string]string{
		"/v1/venues?lng=-74":                         "lat",
		"/v1/venues?lat=40.7":                        "lng",
		"/v1/venues?lat=abc&lng=-74":                 "lat",
		"/v1/venues?lat=95&lng=-74":                  "lat",
		"/v1/venues?lat=40.7&lng=-74&radius_m=50000": "radius_m",
		"/v1/venues?lat=40.7&lng=-74&radius_m=0":     "radius_m",
		"/v1/venues?lat=40.7&lng=-74&limit=201":      "limit",
		"/v1/venues?lat=40.7&lng=-74&day=7":          "day",
		"/v1/venues?lat=40.7&lng=-74&active_at=soon": "active_at",
		"/v1/venues?lat=40.7&lng=-74&cursor=!!":      "cursor",
	}
	for path, param := range cases {
		var body errorJSON
		code := a.get(path, &body)
		if code != http.StatusBadRequest || body.Error.Code != "invalid_param" || !strings.Contains(body.Error.Message, param) {
			t.Errorf("%s: got %d %+v, want 400 invalid_param naming %s", path, code, body, param)
		}
	}
}

func TestUnknownEndpointUsesEnvelope(t *testing.T) {
	a := newAPI(t, time.Now())
	// Not in the contract, so check the envelope directly instead of validating.
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/nope", nil))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"code":"not_found"`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}
