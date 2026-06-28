package notice

import (
	"bytes"
	"strings"
	"testing"
)

func TestActivationFailedDoesNotRequireSecretValues(t *testing.T) {
	var buf bytes.Buffer
	ActivationFailed(&buf, "psql", "production", "missing secret password")

	got := buf.String()
	if !strings.Contains(got, "missing secret password") {
		t.Fatalf("expected missing secret field in notice, got %q", got)
	}
	if strings.Contains(got, "cloak_secret") {
		t.Fatalf("notice leaked Secret Material: %q", got)
	}
}
