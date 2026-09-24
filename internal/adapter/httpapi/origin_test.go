package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOriginSecretProtectsEveryRoute(t *testing.T) {
	handler := RequireOriginSecret(NewHandler(testPinger{}, nil), "an-origin-secret-longer-than-thirty-two-characters")
	for _, tc := range []struct {
		name   string
		values []string
		want   int
	}{
		{name: "missing", want: http.StatusForbidden},
		{name: "wrong", values: []string{"wrong"}, want: http.StatusForbidden},
		{name: "duplicate", values: []string{"an-origin-secret-longer-than-thirty-two-characters", "wrong"}, want: http.StatusForbidden},
		{name: "correct", values: []string{"an-origin-secret-longer-than-thirty-two-characters"}, want: http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/health", nil)
			for _, value := range tc.values {
				request.Header.Add("X-Origin-Secret", value)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d", response.Code, tc.want)
			}
		})
	}
}
