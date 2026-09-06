package shim

import "testing"

func TestDetectInvocation(t *testing.T) {
	tests := []struct {
		argv0    string
		wantMode InvocationMode
		wantCLI  string
	}{
		{argv0: "/usr/local/bin/cloak", wantMode: StandaloneMode},
		{argv0: "/tmp/shims/psql", wantMode: ShimMode, wantCLI: "psql"},
	}

	for _, tt := range tests {
		got := DetectInvocation(tt.argv0)
		if got.Mode != tt.wantMode {
			t.Fatalf("DetectInvocation(%q) mode = %v, want %v", tt.argv0, got.Mode, tt.wantMode)
		}
		if got.ManagedCLI != tt.wantCLI {
			t.Fatalf("DetectInvocation(%q) ManagedCLI = %q, want %q", tt.argv0, got.ManagedCLI, tt.wantCLI)
		}
	}
}
