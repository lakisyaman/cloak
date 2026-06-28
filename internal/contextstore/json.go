package contextstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func ReadConfig(path string) (Config, error) {
	var config Config
	if err := readJSON(path, &config); err != nil {
		if os.IsNotExist(err) {
			return EmptyConfig(), nil
		}
		return Config{}, err
	}
	if config.Version != Version {
		return Config{}, fmt.Errorf("unsupported config version %d", config.Version)
	}
	if config.ManagedCLIs == nil {
		config.ManagedCLIs = map[string]ManagedCLIConfig{}
	}
	return config, nil
}

func ReadState(path string) (State, error) {
	var state State
	if err := readJSON(path, &state); err != nil {
		if os.IsNotExist(err) {
			return EmptyState(), nil
		}
		return State{}, err
	}
	if state.Version != Version {
		return State{}, fmt.Errorf("unsupported state version %d", state.Version)
	}
	if state.ActiveContexts == nil {
		state.ActiveContexts = map[string]string{}
	}
	return state, nil
}

func WriteConfig(path string, config Config) error {
	config.Version = Version
	if config.ManagedCLIs == nil {
		config.ManagedCLIs = map[string]ManagedCLIConfig{}
	}
	return writeJSONAtomic(path, config)
}

func WriteState(path string, state State) error {
	state.Version = Version
	if state.ActiveContexts == nil {
		state.ActiveContexts = map[string]string{}
	}
	return writeJSONAtomic(path, state)
}

func readJSON(path string, dest any) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(contents, dest)
}

func writeJSONAtomic(path string, value any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	contents, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(contents); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}

	_ = syncDir(dir)
	return nil
}

func syncDir(dir string) error {
	dirFile, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = dirFile.Close() }()
	return dirFile.Sync()
}
