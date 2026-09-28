package deviceauth

import (
	"context"
	"strings"

	model "github.com/HyeonTee/agent-control-plane/internal/domain/deviceauth"
	work "github.com/HyeonTee/agent-control-plane/internal/domain/work"
)

// Store is the application port for durable device grants and tokens.
type Store interface {
	StartDevice(context.Context, string, []string) (model.Authorization, error)
	FindDevice(context.Context, string) (model.Authorization, error)
	DecideDevice(context.Context, string, bool) error
	ExchangeDevice(context.Context, string) (model.Tokens, error)
	RefreshDevice(context.Context, string) (model.Tokens, error)
	ListDevices(context.Context) ([]model.Client, error)
	RevokeDevice(context.Context, string) error
}

type Service struct{ Store }

func New(store Store) *Service { return &Service{Store: store} }

func (s *Service) StartDevice(ctx context.Context, label string, scopes []string) (model.Authorization, error) {
	label = strings.TrimSpace(label)
	if label == "" || len(label) > 120 || len(scopes) == 0 || len(scopes) > 2 {
		return model.Authorization{}, work.ErrInvalid
	}
	seen := make(map[string]bool, len(scopes))
	for _, scope := range scopes {
		if (scope != work.ScopeRead && scope != work.ScopeWrite) || seen[scope] {
			return model.Authorization{}, work.ErrInvalid
		}
		seen[scope] = true
	}
	return s.Store.StartDevice(ctx, label, scopes)
}
