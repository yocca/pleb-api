package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecovererReturnsGeneric500(t *testing.T) {
	h := recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("pq: relation \"secret_table\" does not exist")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	if got, want := strings.TrimSpace(rec.Body.String()), `{"error":{"code":"internal","message":"internal error"}}`; got != want {
		t.Fatalf("body %s, want %s", got, want)
	}
}

func TestWriteInternalHidesCause(t *testing.T) {
	rec := httptest.NewRecorder()
	writeInternal(rec, httptest.NewRequest(http.MethodGet, "/", nil), errors.New("SELECT * FROM venues: connection refused"))
	if strings.Contains(rec.Body.String(), "SELECT") || rec.Code != http.StatusInternalServerError {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

func TestErrorEnvelopeShapes(t *testing.T) {
	for status, code := range map[int]string{
		http.StatusBadRequest:          codeInvalidParam,
		http.StatusNotFound:            codeNotFound,
		http.StatusInternalServerError: codeInternal,
		http.StatusServiceUnavailable:  codeUnavailable,
	} {
		rec := httptest.NewRecorder()
		writeError(rec, status, code, "msg")
		want := `{"error":{"code":"` + code + `","message":"msg"}}`
		if rec.Code != status || strings.TrimSpace(rec.Body.String()) != want || rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("%d: got %d %s", status, rec.Code, rec.Body)
		}
	}
}
