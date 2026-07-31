package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/christoph-jerolimov/agentsview-rhdh-extension/internal/rhdh"
	"github.com/christoph-jerolimov/agentsview-rhdh-extension/internal/upstream"
)

// rhdhFlags are the session-scoping flags shared by the rhdh filters,
// forwarded to the upstream `session search` invocation.
type rhdhFlags struct {
	project string
	agent   string
	since   string
	limit   int
	jsonOut bool
}

func (f *rhdhFlags) register(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVar(&f.project, "project", "", "Filter by project name")
	flags.StringVar(&f.agent, "agent", "", "Filter by agent")
	flags.StringVar(&f.since, "since", "",
		"Only sessions active since a relative duration (12h, 14d, 2w) or YYYY-MM-DD")
	flags.IntVar(&f.limit, "limit", 200, "Maximum content matches to scan")
	flags.BoolVar(&f.jsonOut, "json", false, "Emit JSON output")
}

func (f *rhdhFlags) searchOptions() upstream.SearchOptions {
	return upstream.SearchOptions{
		Project: f.project,
		Agent:   f.agent,
		Since:   f.since,
		Limit:   f.limit,
	}
}

// newRhdhCommand builds the `rhdh` analysis subcommand. searcher is nil in
// production (resolved lazily from the upstream binary) and stubbed in
// tests.
func newRhdhCommand(searcher upstream.Searcher) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rhdh",
		Short:   "RHDH session analysis: find sessions by Jira issue or GitHub PR",
		GroupID: groupRhdh,
		Long: "Analyse recorded AgentsView sessions for Red Hat Developer Hub " +
			"workflows: find the sessions that reference a Jira issue, or " +
			"created / referenced a GitHub pull request.",
	}
	cmd.AddCommand(newRhdhJiraCommand(searcher))
	cmd.AddCommand(newRhdhPRCommand(searcher))
	return cmd
}

func resolveSearcher(searcher upstream.Searcher) (upstream.Searcher, error) {
	if searcher != nil {
		return searcher, nil
	}
	return upstream.Resolve()
}

func newRhdhJiraCommand(searcher upstream.Searcher) *cobra.Command {
	var f rhdhFlags
	cmd := &cobra.Command{
		Use:   "jira [ISSUE-KEY...]",
		Short: "Find sessions that reference Jira issues",
		Long: "Search message and tool content across all recorded sessions " +
			"for Jira issue keys (e.g. RHIDP-1234) and list the sessions that " +
			"reference them. Without arguments any Jira-shaped key matches; " +
			"with arguments only the given issues match.",
		Example: "  agentsview-rhdh rhdh jira\n" +
			"  agentsview-rhdh rhdh jira RHIDP-1234\n" +
			"  agentsview-rhdh rhdh jira RHIDP-1234 RHDHBUGS-42 --since 2w --json",
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			keys, err := rhdh.NormalizeJiraKeys(args)
			if err != nil {
				return err
			}
			s, err := resolveSearcher(searcher)
			if err != nil {
				return err
			}
			res, err := s.SearchContent(cmd.Context(),
				rhdh.JiraSearchPattern(keys), f.searchOptions())
			if err != nil {
				return err
			}
			sessions := rhdh.GroupJiraSessions(res.Matches, keys)
			if f.jsonOut {
				return writeJSON(cmd.OutOrStdout(), sessions)
			}
			return printJiraSessions(cmd.OutOrStdout(), sessions)
		},
	}
	f.register(cmd)
	return cmd
}

func newRhdhPRCommand(searcher upstream.Searcher) *cobra.Command {
	var (
		f           rhdhFlags
		repo        string
		createdOnly bool
	)
	cmd := &cobra.Command{
		Use:   "pr [NUMBER | URL]",
		Short: "Find sessions that create or reference GitHub pull requests",
		Long: "Search message and tool content across all recorded sessions " +
			"for GitHub pull request URLs and PR-creation markers (gh pr " +
			"create, the GitHub MCP create_pull_request tool, ...) and list " +
			"the matching sessions. Narrow to one PR with a number plus " +
			"--repo, or with a full PR URL.",
		Example: "  agentsview-rhdh rhdh pr\n" +
			"  agentsview-rhdh rhdh pr --created-only\n" +
			"  agentsview-rhdh rhdh pr 42 --repo redhat-developer/rhdh\n" +
			"  agentsview-rhdh rhdh pr https://github.com/redhat-developer/rhdh/pull/42",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			arg := ""
			if len(args) == 1 {
				arg = args[0]
			}
			filter, err := rhdh.ParsePRFilter(arg, repo)
			if err != nil {
				return err
			}
			s, err := resolveSearcher(searcher)
			if err != nil {
				return err
			}
			res, err := s.SearchContent(cmd.Context(),
				rhdh.PRSearchPattern(), f.searchOptions())
			if err != nil {
				return err
			}
			sessions := rhdh.GroupPRSessions(res.Matches, filter, createdOnly)
			if f.jsonOut {
				return writeJSON(cmd.OutOrStdout(), sessions)
			}
			return printPRSessions(cmd.OutOrStdout(), sessions)
		},
	}
	f.register(cmd)
	cmd.Flags().StringVar(&repo, "repo", "",
		"Only PRs of this repository (owner/name)")
	cmd.Flags().BoolVar(&createdOnly, "created-only", false,
		"Only sessions that created a pull request")
	return cmd
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printJiraSessions(w io.Writer, sessions []rhdh.JiraSession) error {
	if len(sessions) == 0 {
		fmt.Fprintln(w, "No sessions referencing Jira issues found.")
		return nil
	}
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "SESSION\tPROJECT\tAGENT\tLAST SEEN\tMATCHES\tISSUES")
	for _, s := range sessions {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n",
			s.SessionID, s.Project, s.Agent, s.LastSeen, s.MatchCount,
			strings.Join(s.IssueKeys, ", "))
	}
	return tw.Flush()
}

func printPRSessions(w io.Writer, sessions []rhdh.PRSession) error {
	if len(sessions) == 0 {
		fmt.Fprintln(w, "No sessions creating or referencing GitHub pull requests found.")
		return nil
	}
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "SESSION\tPROJECT\tAGENT\tLAST SEEN\tMATCHES\tCREATED\tPULL REQUESTS")
	for _, s := range sessions {
		prs := make([]string, len(s.PRs))
		for i, r := range s.PRs {
			prs[i] = r.String()
		}
		created := ""
		if s.Created {
			created = "yes"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			s.SessionID, s.Project, s.Agent, s.LastSeen, s.MatchCount,
			created, strings.Join(prs, ", "))
	}
	return tw.Flush()
}
