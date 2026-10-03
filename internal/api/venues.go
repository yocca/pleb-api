package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/yocca/pleb-api/internal/store"
)

const (
	defaultRadiusM = 1500
	maxRadiusM     = 25000
	defaultLimit   = 50
	maxLimit       = 200
)

type location struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type happyHour struct {
	DayOfWeek  int16           `json:"day_of_week"`
	Start      *string         `json:"start"`
	End        *string         `json:"end"`
	AllDay     bool            `json:"all_day"`
	Deals      json.RawMessage `json:"deals"`
	Notes      *string         `json:"notes"`
	Confidence float32         `json:"confidence"`
	Verified   bool            `json:"verified"`
	SourceKind string          `json:"source_kind"`
	SourceURL  *string         `json:"source_url"`
	ObservedAt time.Time       `json:"observed_at"`
}

type venue struct {
	ID         uuid.UUID   `json:"id"`
	Name       string      `json:"name"`
	Address    *string     `json:"address"`
	Location   location    `json:"location"`
	Timezone   string      `json:"timezone"`
	ActiveNow  bool        `json:"active_now"`
	HappyHours []happyHour `json:"happy_hours"`
}

type searchVenue struct {
	venue
	DistanceM float64 `json:"distance_m"`
}

type venueDetail struct {
	venue
	Website     *string   `json:"website"`
	Phone       *string   `json:"phone"`
	UpdatedAt   time.Time `json:"updated_at"`
	Attribution []string  `json:"attribution"`
}

type searchResponse struct {
	Venues     []searchVenue `json:"venues"`
	NextCursor *string       `json:"next_cursor"`
}

// cursor is the keyset position after the last venue of a page.
type cursor struct {
	Distance float64   `json:"d"`
	ID       uuid.UUID `json:"id"`
}

func (c cursor) encode() string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (cursor, error) {
	var c cursor
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	return c, err
}

type paramError struct{ msg string }

func (e paramError) Error() string { return e.msg }

func badParam(format string, args ...any) error { return paramError{fmt.Sprintf(format, args...)} }

func parseSearch(q url.Values, now time.Time) (store.SearchVenuesParams, error) {
	p := store.SearchVenuesParams{EvaluatedAt: now, RadiusM: defaultRadiusM}

	var err error
	if p.Lat, err = requiredFloat(q, "lat", -90, 90); err != nil {
		return p, err
	}
	if p.Lng, err = requiredFloat(q, "lng", -180, 180); err != nil {
		return p, err
	}
	radius, err := optionalInt(q, "radius_m", 1, maxRadiusM, defaultRadiusM)
	if err != nil {
		return p, err
	}
	p.RadiusM = float64(radius)

	limit, err := optionalInt(q, "limit", 1, maxLimit, defaultLimit)
	if err != nil {
		return p, err
	}
	p.RowLimit = int32(limit) + 1 // one extra row tells us whether another page exists

	if q.Has("day") {
		day, err := optionalInt(q, "day", 0, 6, 0)
		if err != nil {
			return p, err
		}
		d := int16(day)
		p.Day = &d
	}
	if v := q.Get("active_at"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return p, badParam("active_at must be an RFC 3339 timestamp")
		}
		p.ActiveAt = &t
		p.EvaluatedAt = t
	}
	if v := q.Get("cursor"); v != "" {
		c, err := decodeCursor(v)
		if err != nil {
			return p, badParam("cursor is not valid")
		}
		p.AfterDistance, p.AfterID = &c.Distance, &c.ID
	}
	return p, nil
}

func requiredFloat(q url.Values, name string, lo, hi float64) (float64, error) {
	v := q.Get(name)
	if v == "" {
		return 0, badParam("%s is required", name)
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(f) || f < lo || f > hi {
		return 0, badParam("%s must be a number between %g and %g", name, lo, hi)
	}
	return f, nil
}

func optionalInt(q url.Values, name string, lo, hi, def int) (int, error) {
	v := q.Get(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < lo || n > hi {
		return 0, badParam("%s must be an integer between %d and %d", name, lo, hi)
	}
	return n, nil
}

func (s *Server) searchVenues(w http.ResponseWriter, r *http.Request) {
	params, err := parseSearch(r.URL.Query(), s.now())
	if err != nil {
		writeError(w, http.StatusBadRequest, codeInvalidParam, err.Error())
		return
	}
	rows, err := s.queries.SearchVenues(r.Context(), params)
	if err != nil {
		writeInternal(w, r, err)
		return
	}

	resp := searchResponse{Venues: []searchVenue{}}
	limit := int(params.RowLimit) - 1
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next := cursor{Distance: last.DistanceM, ID: last.ID}.encode()
		resp.NextCursor = &next
	}

	ids := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	hours, err := s.happyHoursByVenue(r, ids)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	for _, row := range rows {
		resp.Venues = append(resp.Venues, searchVenue{
			venue: venue{
				ID: row.ID, Name: row.Name, Address: row.Address,
				Location:   location{Lat: row.Lat, Lng: row.Lng},
				Timezone:   row.Timezone,
				ActiveNow:  row.ActiveNow,
				HappyHours: orEmpty(hours[row.ID]),
			},
			DistanceM: math.Round(row.DistanceM*10) / 10,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) getVenue(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "venue not found")
		return
	}
	row, err := s.queries.GetVenue(r.Context(), store.GetVenueParams{ID: id, EvaluatedAt: s.now()})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, codeNotFound, "venue not found")
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	hours, err := s.happyHoursByVenue(r, []uuid.UUID{id})
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, venueDetail{
		venue: venue{
			ID: row.ID, Name: row.Name, Address: row.Address,
			Location:   location{Lat: row.Lat, Lng: row.Lng},
			Timezone:   row.Timezone,
			ActiveNow:  row.ActiveNow,
			HappyHours: orEmpty(hours[row.ID]),
		},
		Website:     row.Website,
		Phone:       row.Phone,
		UpdatedAt:   row.UpdatedAt,
		Attribution: orEmpty(row.Attribution),
	})
}

func (s *Server) happyHoursByVenue(r *http.Request, ids []uuid.UUID) (map[uuid.UUID][]happyHour, error) {
	out := make(map[uuid.UUID][]happyHour, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.queries.VisibleHappyHoursForVenues(r.Context(), ids)
	if err != nil {
		return nil, err
	}
	for _, h := range rows {
		deals := h.Deals
		if len(deals) == 0 {
			deals = json.RawMessage("[]")
		}
		out[h.VenueID] = append(out[h.VenueID], happyHour{
			DayOfWeek:  h.DayOfWeek,
			Start:      clock(h.StartTime),
			End:        clock(h.EndTime),
			AllDay:     h.AllDay,
			Deals:      deals,
			Notes:      h.Notes,
			Confidence: h.Confidence,
			Verified:   h.Verified,
			SourceKind: h.SourceKind,
			SourceURL:  h.SourceUrl,
			ObservedAt: h.ObservedAt,
		})
	}
	return out, nil
}

// clock formats a Postgres time as HH:MM.
func clock(t pgtype.Time) *string {
	if !t.Valid {
		return nil
	}
	mins := t.Microseconds / int64(time.Minute/time.Microsecond)
	s := fmt.Sprintf("%02d:%02d", mins/60, mins%60)
	return &s
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
