package secrets

import "testing"

func TestNewRefUsesCanonicalKeyringMapping(t *testing.T) {
	ref := NewRef("psql", "production", "password")

	if ref.Store != "os" {
		t.Fatalf("expected store os, got %q", ref.Store)
	}
	if ref.Service != "cloak" {
		t.Fatalf("expected service cloak, got %q", ref.Service)
	}
	if ref.User != "psql/production/password" {
		t.Fatalf("expected user psql/production/password, got %q", ref.User)
	}
}
