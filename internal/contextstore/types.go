package contextstore

import "github.com/lakisyaman/cloak/internal/secrets"

const Version = 1

type Config struct {
	Version     int                         `json:"version"`
	ManagedCLIs map[string]ManagedCLIConfig `json:"managedClis"`
}

type ManagedCLIConfig struct {
	Contexts map[string]Context `json:"contexts"`
}

type Context struct {
	Metadata map[string]any               `json:"metadata"`
	Secrets  map[string]secrets.SecretRef `json:"secrets,omitempty"`
}

type State struct {
	Version        int               `json:"version"`
	ActiveContexts map[string]string `json:"activeContexts"`
}

func EmptyConfig() Config {
	return Config{Version: Version, ManagedCLIs: map[string]ManagedCLIConfig{}}
}

func EmptyState() State {
	return State{Version: Version, ActiveContexts: map[string]string{}}
}
