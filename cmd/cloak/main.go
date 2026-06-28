package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/lakisyaman/cloak/internal/app"
)

// Build metadata. version is overridden via -ldflags by GoReleaser and the
// Makefile build target; commit and date are optional and fall back to the
// build info embedded by the Go toolchain.
var (
	version = "dev"
	commit  string
	date    string
)

func main() {
	v, c, d := buildInfo()
	app.SetBuildInfo(c, d)
	if err := app.ExecuteInvocation(v, os.Args[0], os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// buildInfo prefers -ldflags values and falls back to the module version and
// VCS metadata embedded by the Go toolchain, so `go install ...@vX.Y.Z` and a
// plain `go build` still report something meaningful.
func buildInfo() (v, c, d string) {
	v, c, d = version, commit, date
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return v, c, d
	}
	if v == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		v = info.Main.Version
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if c == "" {
				c = setting.Value
			}
		case "vcs.time":
			if d == "" {
				d = setting.Value
			}
		}
	}
	return v, c, d
}
