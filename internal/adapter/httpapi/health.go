package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/HyeonTee/agent-control-plane/api"
	appwork "github.com/HyeonTee/agent-control-plane/internal/application/work"
	model "github.com/HyeonTee/agent-control-plane/internal/domain/work"
	"github.com/HyeonTee/agent-control-plane/internal/requestid"
)

type Pinger interface {
	Ping(context.Context) error
}

type Backend interface {
	appwork.Store
	Authenticate(context.Context, string) (model.Actor, error)
	AuditFailure(context.Context, model.Actor, string, int) error
}

func NewHandler(db Pinger, backend Backend) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write(api.OpenAPI)
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if backend != nil {
		registerWorkRoutes(mux, backend)
	}
	return requestid.Middleware(mux)
}
