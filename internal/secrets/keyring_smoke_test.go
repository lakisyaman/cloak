package secrets

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestKeyringStoreRoundTrip(t *testing.T) {
	if os.Getenv("CLOAK_TEST_KEYRING") != "1" {
		t.Skip("set CLOAK_TEST_KEYRING=1 to run the OS keyring smoke test")
	}

	store := KeyringStore{}
	ref := SecretRef{
		Store:   "os",
		Service: "cloak-test",
		User:    fmt.Sprintf("roundtrip/%d", time.Now().UnixNano()),
	}
	defer func() { _ = store.Delete(ref) }()

	const want = "secret-value"
	if err := store.Set(ref, want); err != nil {
		t.Fatalf("set keyring secret: %v", err)
	}

	got, err := store.Get(ref)
	if err != nil {
		t.Fatalf("get keyring secret: %v", err)
	}
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
