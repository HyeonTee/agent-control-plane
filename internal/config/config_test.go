package config

import "testing"

func TestProductionRequiresOriginSecret(t *testing.T) {
	t.Setenv("HUB_DATABASE_URL", "postgres://example")
	t.Setenv("HUB_ENV", "production")
	t.Setenv("HUB_ORIGIN_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("production accepted a missing origin secret")
	}
	t.Setenv("HUB_ORIGIN_SECRET", "short")
	if _, err := Load(); err == nil {
		t.Fatal("production accepted a short origin secret")
	}
	t.Setenv("HUB_ORIGIN_SECRET", "a-secret-longer-than-thirty-two-characters")
	if _, err := Load(); err != nil {
		t.Fatalf("production rejected a valid origin secret: %v", err)
	}
}

func TestUnknownEnvironmentFailsClosed(t *testing.T) {
	t.Setenv("HUB_DATABASE_URL", "postgres://example")
	t.Setenv("HUB_ENV", "prod")
	if _, err := Load(); err == nil {
		t.Fatal("unknown environment accepted")
	}
}
