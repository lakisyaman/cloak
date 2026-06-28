package adapters

import "github.com/lakisyaman/cloak/internal/contextstore"

type Invocation struct {
	Args []string
	Env  []string
}

type ActivatedInvocation struct {
	Args []string
	Env  []string
}

type Adapter interface {
	Name() string
	DetectExplicitConnectionInput(args []string, env []string) bool
	Activate(invocation Invocation, ctx contextstore.Context, secretValues map[string]string) (ActivatedInvocation, error)
}
