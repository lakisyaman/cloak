package connectors

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestLocalSnapshotRequiresExplicitRefresh(t *testing.T) {
	source := filepath.Join(t.TempDir(), "acme.yaml")
	if err := os.WriteFile(source, []byte(customYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &Store{Dir: filepath.Join(t.TempDir(), "connectors")}
	if names, err := store.Names(); err != nil || len(names) != 0 {
		t.Fatalf("fresh store has Connectors: %v %v", names, err)
	}
	record, _, err := store.Acquire(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Write(record); err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(customYAML, "ACME_ENDPOINT", "ACME_ADDRESS", 1)
	if err = os.WriteFile(source, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	def, err := store.Get("acme-cli")
	if err != nil {
		t.Fatal(err)
	}
	if def.Fields["endpoint"].Inject.Env != "ACME_ENDPOINT" {
		t.Fatal("live source leaked into installed definition")
	}
	record, _, err = store.Acquire(context.Background(), record.Source)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Write(record); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(source); err != nil {
		t.Fatal(err)
	}
	def, err = store.Get("acme-cli")
	if err != nil {
		t.Fatal(err)
	}
	if def.Fields["endpoint"].Inject.Env != "ACME_ADDRESS" {
		t.Fatal("explicit refresh did not take effect")
	}
	info, err := os.Stat(filepath.Join(store.Dir, "acme-cli.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("non-private record permissions: %v", info.Mode())
	}
}

func TestRegistryAcquisitionIsOnDemandAndRuntimeIsOffline(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/acme-cli.yaml" {
			t.Errorf("unexpected registry path %s", r.URL.Path)
		}
		fmt.Fprint(w, customYAML)
	}))
	defer server.Close()
	store := &Store{Dir: t.TempDir(), RegistryURL: server.URL}
	if requests.Load() != 0 {
		t.Fatal("registry contacted before acquisition")
	}
	record, _, err := store.Acquire(context.Background(), "@cloak/acme-cli")
	if err != nil {
		t.Fatal(err)
	}
	if record.Source != "@cloak/acme-cli" {
		t.Fatal("source not retained")
	}
	if err = store.Write(record); err != nil {
		t.Fatal(err)
	}
	server.Close()
	if _, err = store.Get("acme-cli"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatal("runtime fetched registry")
	}
	for _, source := range []string{"@other/acme-cli", "@cloak/../psql", "https://host/config.yaml", "missing.yaml", "config.json"} {
		if _, _, err = store.Acquire(context.Background(), source); err == nil {
			t.Fatalf("accepted bad source %s", source)
		}
	}
}

func TestRegistryRejectsMismatchedAndOversizedDefinitions(t *testing.T) {
	for _, body := range []string{strings.Replace(customYAML, "acme-cli", "other-cli", 1), strings.Repeat("x", MaxDefinitionBytes+1)} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		store := &Store{Dir: t.TempDir(), RegistryURL: server.URL}
		if _, _, err := store.Acquire(context.Background(), "@cloak/acme-cli"); err == nil {
			t.Fatal("accepted invalid registry response")
		}
		server.Close()
	}
}

func TestFailedRefreshPreservesInstalledRecord(t *testing.T) {
	store := &Store{Dir: t.TempDir()}
	record := Record{Version: 1, Source: "@cloak/acme-cli", YAML: customYAML}
	if err := store.Write(record); err != nil {
		t.Fatal(err)
	}
	record.YAML = "invalid YAML"
	if err := store.Write(record); err == nil {
		t.Fatal("accepted invalid update")
	}
	if _, err := store.Get("acme-cli"); err != nil {
		t.Fatal("failed update damaged previous copy", err)
	}
}
