package shim

import "path/filepath"

const StandaloneName = "cloak"

type InvocationMode int

const (
	StandaloneMode InvocationMode = iota
	ShimMode
)

type Invocation struct {
	Mode       InvocationMode
	Name       string
	ManagedCLI string
}

func DetectInvocation(argv0 string) Invocation {
	name := filepath.Base(argv0)
	if name == StandaloneName {
		return Invocation{Mode: StandaloneMode, Name: name}
	}
	return Invocation{Mode: ShimMode, Name: name, ManagedCLI: name}
}
