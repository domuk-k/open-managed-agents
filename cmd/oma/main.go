package main

import (
	"fmt"
	"os"

	"github.com/domuk-k/open-managed-agents/cli"
)

// version is set via ldflags at build time by goreleaser.
var version = "dev"

func main() {
	cli.SetVersion(version)
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
