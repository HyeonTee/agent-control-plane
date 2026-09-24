package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type testPinger struct{ err error }

func (p testPinger) Ping(context.Context) error { return p.err }

func TestHealthDoesNotDependOnDatabase(t *testing.T) {
	r := httptest.NewRecorder()
	NewHandler(testPinger{err: errors.New("down")}).ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/health", nil))
	if r.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", r.Code)
	}
}

func TestReadyReportsDatabaseState(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"up", nil, http.StatusNoContent},
		{"down", errors.New("down"), http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRecorder()
			NewHandler(testPinger{err: tc.err}).ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/ready", nil))
			if r.Code != tc.want {
				t.Fatalf("status = %d, want %d", r.Code, tc.want)
			}
		})
	}
}
