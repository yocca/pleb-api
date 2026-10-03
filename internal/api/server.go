// Package api serves pleb-api's read-only HTTP endpoints, as described in openapi.yaml.
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/yocca/pleb-api/internal/store"
)

// Pinger reports whether the database is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Server holds the dependencies shared by the handlers.
type Server struct {
	queries *store.Queries
	db      Pinger
	now     func() time.Time
}

// New builds the HTTP handler. db must also satisfy store.DBTX (a *pgxpool.Pool does).
func New(db interface {
	Pinger
	store.DBTX
}) *Server {
	return &Server{queries: store.New(db), db: db, now: time.Now}
}

// WithClock overrides the clock used for active_now; tests use it to pin "now".
func (s *Server) WithClock(now func() time.Time) *Server {
	s.now = now
	return s
}

// Handler returns the routed HTTP handler.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Logger, recoverer)

	r.Get("/healthz", s.health)
	r.Get("/v1/venues", s.searchVenues)
	r.Get("/v1/venues/{id}", s.getVenue)

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, codeNotFound, "no such endpoint")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, codeNotFound, "no such endpoint")
	})
	return r
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, codeUnavailable, "database unreachable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
