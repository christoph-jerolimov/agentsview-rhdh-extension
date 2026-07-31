// Command agentsview-rhdh wraps the AgentsView CLI (go.kenn.io/agentsview)
// and adds RHDH-specific session analysis subcommands.
package main

import (
	"os"

	"github.com/christoph-jerolimov/agentsview-rhdh-extension/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
