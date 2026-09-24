package requestid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

type key struct{}

func From(ctx context.Context) string {
	id, _ := ctx.Value(key{}).(string)
	return id
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			http.Error(w, "request failed", http.StatusInternalServerError)
			return
		}
		id := hex.EncodeToString(random[:])
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), key{}, id)))
	})
}
