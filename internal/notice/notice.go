package notice

import (
	"fmt"
	"io"
)

func Activated(w io.Writer, managedCLI, contextName string) {
	fmt.Fprintf(w, "cloak: activated %s context %s\n", managedCLI, contextName)
}

func NoActiveContext(w io.Writer, managedCLI string) {
	fmt.Fprintf(w, "cloak: no active context for %s; running without activation\n", managedCLI)
}

func ExplicitConnectionInput(w io.Writer, managedCLI string) {
	fmt.Fprintf(w, "cloak: explicit connection input detected for %s; running without activation\n", managedCLI)
}

func ActivationFailed(w io.Writer, managedCLI, contextName, reason string) {
	fmt.Fprintf(w, "cloak: failed to activate %s context %s: %s\n", managedCLI, contextName, reason)
}
