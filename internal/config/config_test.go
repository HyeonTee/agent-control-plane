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

func TestDeviceAuthConfigurationRequiresCompleteSecureProductionSettings(t *testing.T) {
	t.Setenv("HUB_DATABASE_URL", "postgres://example")
	t.Setenv("HUB_ENV", "production")
	t.Setenv("HUB_ORIGIN_SECRET", "a-secret-longer-than-thirty-two-characters")
	t.Setenv("HUB_GITHUB_CLIENT_ID", "client")
	t.Setenv("HUB_GITHUB_CLIENT_SECRET", "secret")
	t.Setenv("HUB_GITHUB_OWNER_ID", "42")
	t.Setenv("HUB_PUBLIC_URL", "http://agent.gwinam.com")
	if _, err := Load(); err == nil {
		t.Fatal("production accepted an insecure approval URL")
	}
	t.Setenv("HUB_PUBLIC_URL", "https://agent.gwinam.com")
	if _, err := Load(); err != nil {
		t.Fatalf("valid device auth configuration rejected: %v", err)
	}
	t.Setenv("HUB_GITHUB_OWNER_ID", "")
	if _, err := Load(); err == nil {
		t.Fatal("partial device auth configuration accepted")
	}
}
