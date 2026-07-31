package cli

import (
	"github.com/spf13/cobra"

	"github.com/christoph-jerolimov/agentsview-rhdh-extension/internal/upstream"
)

const (
	groupRhdh     = "rhdh"
	groupUpstream = "agentsview"
)

// upstreamCommands mirrors the top-level command set of AgentsView
// v0.39.0 (cmd/agentsview/cli.go). Each entry becomes a passthrough
// subcommand that forwards its full argument list to the upstream binary,
// so everything AgentsView provides stays reachable from this CLI.
var upstreamCommands = []struct {
	name, short string
}{
	{"serve", "Start server"},
	{"daemon", "Manage the background server"},
	{"sync", "Sync session data without serving"},
	{"prune", "Delete sessions matching filters"},
	{"update", "Check for and install updates"},
	{"token-use", "Show token usage for a session (JSON)"},
	{"import", "Import conversations"},
	{"export", "Export local archive data"},
	{"projects", "List projects with session counts"},
	{"health", "Show session health and signals"},
	{"usage", "Token cost tracking and reporting"},
	{"activity", "Activity and concurrency reporting"},
	{"pg", "PostgreSQL sync and serve commands"},
	{"duckdb", "DuckDB sync and serve commands"},
	{"embeddings", "Manage the semantic search embedding index"},
	{"session", "Programmatic access to session data"},
	{"mcp", "Run an MCP server exposing read-only session retrieval tools"},
	{"recall", "Build and inspect recalled knowledge from past sessions"},
	{"stats", "Window-scoped workspace analytics"},
	{"parse-diff", "Re-parse session files and diff against the archive"},
	{"classifier", "Manage the automated-session classifier"},
	{"secrets", "Scan for and list detected secret leaks"},
	{"skills", "Install and list AgentsView skills for coding-agent harnesses"},
	{"doctor", "Collect support diagnostics"},
	{"openapi", "Print OpenAPI 3.1 schema"},
	{"agentsview-version", "Show upstream AgentsView version information"},
}

// upstreamName maps a passthrough command back to the upstream command it
// invokes. Only `version` is renamed to avoid clashing with our own.
func upstreamName(name string) string {
	if name == "agentsview-version" {
		return "version"
	}
	return name
}

func addPassthroughCommands(root *cobra.Command) {
	for _, c := range upstreamCommands {
		root.AddCommand(newPassthroughCommand(c.name, c.short))
	}
}

func newPassthroughCommand(name, short string) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		Short:              short + " (agentsview passthrough)",
		GroupID:            groupUpstream,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, err := upstream.Resolve()
			if err != nil {
				return err
			}
			code := runner.Exec(cmd.Context(),
				append([]string{upstreamName(name)}, args...)...)
			if code != 0 {
				return exitCodeError{code: code}
			}
			return nil
		},
	}
}
