package adapters

import "testing"

func TestSupportedManagedCLIsUsesSingleSourceOfTruth(t *testing.T) {
	managedCLIs := SupportedManagedCLIs()
	if len(managedCLIs) != 3 {
		t.Fatalf("expected three supported Managed CLIs, got %d: %#v", len(managedCLIs), managedCLIs)
	}
	for _, managedCLI := range managedCLIs {
		if !IsSupported(managedCLI) {
			t.Fatalf("SupportedManagedCLIs returned %q, but IsSupported rejected it", managedCLI)
		}
	}

	managedCLIs[0] = "mutated"
	if SupportedManagedCLIs()[0] == "mutated" {
		t.Fatalf("SupportedManagedCLIs exposed mutable package state")
	}
}
