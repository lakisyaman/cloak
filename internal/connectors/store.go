package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const OfficialRegistryURL = "https://raw.githubusercontent.com/lakisyaman/cloak/main/registry"

// Record is one atomic snapshot: source provenance and the exact YAML acquired
// from that source. Execution never reopens Source.
type Record struct {
	Version int    `json:"version"`
	Source  string `json:"source"`
	YAML    string `json:"definition"`
}

type Store struct {
	Dir    string
	Client *http.Client
	// RegistryURL is injectable for tests; the CLI only uses the official registry.
	RegistryURL string
}

func (s *Store) Acquire(ctx context.Context, source string) (Record, *Definition, error) {
	var data []byte
	var err error
	expected := ""
	if strings.HasPrefix(source, "@") {
		name, ok := strings.CutPrefix(source, "@cloak/")
		if !ok || ValidateCommand(name) != nil {
			return Record{}, nil, fmt.Errorf("registry sources must use @cloak/<cli>")
		}
		expected = name
		base := s.RegistryURL
		if base == "" {
			base = OfficialRegistryURL
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/"+name+".yaml", nil)
		if err != nil {
			return Record{}, nil, fmt.Errorf("invalid registry URL")
		}
		client := s.Client
		if client == nil {
			client = &http.Client{Timeout: 20 * time.Second}
		}
		response, err := client.Do(request)
		if err != nil {
			return Record{}, nil, fmt.Errorf("could not download Connector %s", source)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return Record{}, nil, fmt.Errorf("registry returned HTTP %d for %s", response.StatusCode, source)
		}
		data, err = io.ReadAll(io.LimitReader(response.Body, MaxDefinitionBytes+1))
		if err != nil {
			return Record{}, nil, fmt.Errorf("could not read Connector download")
		}
	} else {
		ext := strings.ToLower(filepath.Ext(source))
		if ext != ".yaml" && ext != ".yml" {
			return Record{}, nil, fmt.Errorf("local Connector sources must be .yaml or .yml files")
		}
		source, err = filepath.Abs(source)
		if err != nil {
			return Record{}, nil, err
		}
		file, err := os.Open(source)
		if err != nil {
			return Record{}, nil, fmt.Errorf("open local Connector: %w", err)
		}
		defer file.Close()
		data, err = io.ReadAll(io.LimitReader(file, MaxDefinitionBytes+1))
		if err != nil {
			return Record{}, nil, fmt.Errorf("read local Connector: %w", err)
		}
	}
	def, err := Parse(data)
	if err != nil {
		return Record{}, nil, err
	}
	if expected != "" && def.Command != expected {
		return Record{}, nil, fmt.Errorf("registry Connector command does not match requested name")
	}
	return Record{Version: 1, Source: source, YAML: string(data)}, def, nil
}

func (s *Store) Read(name string) (Record, *Definition, error) {
	if err := ValidateCommand(name); err != nil {
		return Record{}, nil, err
	}
	file, err := os.Open(filepath.Join(s.Dir, name+".json"))
	if os.IsNotExist(err) {
		return Record{}, nil, fmt.Errorf("Connector %s is not installed; run cloak connector add <source>", name)
	}
	if err != nil {
		return Record{}, nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxDefinitionBytes*6+4096))
	if err != nil {
		return Record{}, nil, err
	}
	var record Record
	if json.Unmarshal(data, &record) != nil || record.Version != 1 {
		return Record{}, nil, fmt.Errorf("invalid installed Connector %s; run cloak connector update %s", name, name)
	}
	def, err := Parse([]byte(record.YAML))
	if err != nil {
		return Record{}, nil, fmt.Errorf("installed Connector %s: %w", name, err)
	}
	if def.Command != name {
		return Record{}, nil, fmt.Errorf("installed Connector identity mismatch")
	}
	return record, def, nil
}

func (s *Store) Get(name string) (*Definition, error) { _, def, err := s.Read(name); return def, err }

func (s *Store) Names() ([]string, error) {
	entries, err := os.ReadDir(s.Dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			name := strings.TrimSuffix(entry.Name(), ".json")
			if ValidateCommand(name) == nil {
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names, nil
}

func (s *Store) Write(record Record) error {
	def, err := Parse([]byte(record.YAML))
	if err != nil {
		return err
	}
	if record.Version != 1 || record.Source == "" {
		return fmt.Errorf("invalid Connector record")
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(s.Dir, ".connector-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err = file.Write(append(data, '\n')); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Rename(file.Name(), filepath.Join(s.Dir, def.Command+".json")); err != nil {
		return err
	}
	if dir, err := os.Open(s.Dir); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func (s *Store) Remove(name string) error {
	if err := ValidateCommand(name); err != nil {
		return err
	}
	err := os.Remove(filepath.Join(s.Dir, name+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
