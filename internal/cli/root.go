// Package cli assembles the agentsview-rhdh command tree: passthrough
// commands mirroring the upstream AgentsView CLI plus the rhdh analysis
// subcommand.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is stamped via -ldflags at release time.
var Version = "dev"

// AgentsviewVersion is the upstream AgentsView release this extension is
// pinned to (see the tool directive in go.mod).
const AgentsviewVersion = "v0.39.0"

// exitCodeError carries an exit code from a passthrough command so the
// upstream binary's exit status is preserved.
type exitCodeError struct{ code int }

func (e exitCodeError) Error() string { return fmt.Sprintf("exit code %d", e.code) }

// NewRootCommand builds the full command tree.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "agentsview-rhdh",
		Short: "AgentsView with RHDH session analysis",
		Long: "agentsview-rhdh wraps the AgentsView CLI (go.kenn.io/agentsview " +
			AgentsviewVersion + ") and adds RHDH-specific analysis of recorded " +
			"AI agent sessions.\n\n" +
			"All upstream AgentsView commands are exposed as passthrough " +
			"subcommands; `rhdh` adds filters for sessions referencing Jira " +
			"issues or GitHub pull requests.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddGroup(
		&cobra.Group{ID: groupRhdh, Title: "RHDH Commands:"},
		&cobra.Group{ID: groupUpstream, Title: "AgentsView Commands (passthrough):"},
	)
	root.SetCompletionCommandGroupID(groupUpstream)
	root.SetHelpCommandGroupID(groupUpstream)

	root.AddCommand(newRhdhCommand(nil))
	root.AddCommand(newVersionCommand())
	addPassthroughCommands(root)
	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Show agentsview-rhdh and pinned AgentsView versions",
		GroupID: groupRhdh,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(),
				"agentsview-rhdh %s (agentsview %s)\n",
				Version, AgentsviewVersion)
		},
	}
}

// Main runs the CLI and returns the process exit code.
func Main(args []string) int {
	root := NewRootCommand()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		if exitErr, ok := err.(exitCodeError); ok {
			return exitErr.code
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}
