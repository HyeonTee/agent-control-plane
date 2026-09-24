package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
)

// RequireOriginSecret rejects requests that did not pass through the configured
// CloudFront distribution. The origin security group is an additional boundary.
func RequireOriginSecret(next http.Handler, secret string) http.Handler {
	expected := sha256.Sum256([]byte(secret))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := r.Header.Values("X-Origin-Secret")
		if len(values) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		actual := sha256.Sum256([]byte(values[0]))
		if subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
