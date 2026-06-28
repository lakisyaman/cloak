package app

import (
	"os"

	"github.com/lakisyaman/cloak/internal/adapters"
	"github.com/lakisyaman/cloak/internal/contextstore"
	"github.com/lakisyaman/cloak/internal/doctor"
	"github.com/lakisyaman/cloak/internal/secrets"
	"github.com/lakisyaman/cloak/internal/shim"
)

type ContextStore interface {
	ReadConfig() (contextstore.Config, error)
	ReadState() (contextstore.State, error)
	WriteConfig(contextstore.Config) error
	WriteState(contextstore.State) error
}

type CommandEnv struct {
	Paths           contextstore.Paths
	Store           ContextStore
	Secrets         secrets.Store
	Resolver        RealCommandResolver
	Adapters        AdapterRegistry
	PathEnv         string
	CloakBinaryPath string
}

func DefaultCommandEnv() (CommandEnv, error) {
	paths, err := contextstore.DefaultPaths()
	if err != nil {
		return CommandEnv{}, err
	}
	cloakBinary, err := os.Executable()
	if err != nil {
		return CommandEnv{}, err
	}
	store := fileContextStore{configPath: paths.ConfigFile, statePath: paths.StateFile}
	return CommandEnv{
		Paths:           paths,
		Store:           store,
		Secrets:         secrets.KeyringStore{},
		Resolver:        shim.RealCommandResolver{PathEnv: os.Getenv("PATH"), CloakBinaryPath: cloakBinary},
		Adapters:        adapterRegistryFunc(adapters.Get),
		PathEnv:         os.Getenv("PATH"),
		CloakBinaryPath: cloakBinary,
	}, nil
}

func normalizeCommandEnv(env CommandEnv) CommandEnv {
	defaults, err := DefaultCommandEnv()
	if err != nil {
		return env
	}
	if env.Paths.Dir == "" {
		env.Paths = defaults.Paths
	}
	if env.Store == nil {
		env.Store = defaults.Store
	}
	if env.Secrets == nil {
		env.Secrets = defaults.Secrets
	}
	if env.Resolver == nil {
		env.Resolver = defaults.Resolver
	}
	if env.Adapters == nil {
		env.Adapters = defaults.Adapters
	}
	if env.PathEnv == "" {
		env.PathEnv = defaults.PathEnv
	}
	if env.CloakBinaryPath == "" {
		env.CloakBinaryPath = defaults.CloakBinaryPath
	}
	return env
}

func doctorOptionsFromEnv(env CommandEnv) doctor.Options {
	return doctor.Options{
		Paths:           env.Paths,
		PathEnv:         env.PathEnv,
		CloakBinaryPath: env.CloakBinaryPath,
		Supported:       doctor.SupportedCLIFunc(adapters.IsSupported),
		SupportedNames:  adapters.SupportedManagedCLIs(),
		SecretChecker:   doctor.KeyringSecretChecker{Store: env.Secrets},
	}
}

type fileContextStore struct {
	configPath string
	statePath  string
}

func (store fileContextStore) ReadConfig() (contextstore.Config, error) {
	return contextstore.ReadConfig(store.configPath)
}

func (store fileContextStore) ReadState() (contextstore.State, error) {
	return contextstore.ReadState(store.statePath)
}

func (store fileContextStore) WriteConfig(config contextstore.Config) error {
	return contextstore.WriteConfig(store.configPath, config)
}

func (store fileContextStore) WriteState(state contextstore.State) error {
	return contextstore.WriteState(store.statePath, state)
}
