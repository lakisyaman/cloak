package main

import (
	"fmt"
	"os"

	"cloak/internal/app"
)

var version = "dev"

func main() {
	if err := app.ExecuteInvocation(version, os.Args[0], os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
