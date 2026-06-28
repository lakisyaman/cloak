package contextstore

import (
	"os"
	"path/filepath"
)

type Paths struct {
	Dir        string
	ConfigFile string
	StateFile  string
	ShimDir    string
}

func DefaultPaths() (Paths, error) {
	baseDir, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, err
	}

	dir := filepath.Join(baseDir, "cloak")
	return Paths{
		Dir:        dir,
		ConfigFile: filepath.Join(dir, "config.json"),
		StateFile:  filepath.Join(dir, "state.json"),
		ShimDir:    filepath.Join(dir, "shims"),
	}, nil
}
