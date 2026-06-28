package doctor

import (
	"strings"
	"testing"

	"cloak/internal/contextstore"
)

func TestCheckStateConsistencyReportsDanglingActiveContext(t *testing.T) {
	config := contextstore.Config{
		Version: contextstore.Version,
		ManagedCLIs: map[string]contextstore.ManagedCLIConfig{
			"psql": {Contexts: map[string]contextstore.Context{}},
		},
	}
	state := contextstore.State{
		Version:        contextstore.Version,
		ActiveContexts: map[string]string{"psql": "production"},
	}

	findings := CheckStateConsistency(config, state, SupportedCLIFunc(func(managedCLI string) bool {
		return managedCLI == "psql"
	}))

	if !hasFinding(findings, SeverityError, "active context psql/production does not exist") {
		t.Fatalf("expected dangling active context error, got %#v", findings)
	}
}

func hasFinding(findings []Finding, severity Severity, contains string) bool {
	for _, finding := range findings {
		if finding.Severity == severity && strings.Contains(finding.Message, contains) {
			return true
		}
	}
	return false
}
