package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/christoph-jerolimov/agentsview-rhdh-extension/internal/rhdh"
	"github.com/christoph-jerolimov/agentsview-rhdh-extension/internal/upstream"
)

// stubSearcher records the pattern/options it was called with and returns
// canned matches.
type stubSearcher struct {
	pattern string
	opts    upstream.SearchOptions
	result  *upstream.SearchResult
	err     error
}

func (s *stubSearcher) SearchContent(_ context.Context, pattern string, opts upstream.SearchOptions) (*upstream.SearchResult, error) {
	s.pattern = pattern
	s.opts = opts
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

func runRhdh(t *testing.T, stub *stubSearcher, args ...string) (string, error) {
	t.Helper()
	cmd := newRhdhCommand(stub)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestRhdhJiraHumanOutput(t *testing.T) {
	stub := &stubSearcher{result: &upstream.SearchResult{Matches: []upstream.ContentMatch{
		{SessionID: "sess-1", Project: "rhdh", Agent: "claude-code",
			Timestamp: "2026-07-01T10:00:00Z", Snippet: "fixing RHIDP-1234"},
	}}}
	out, err := runRhdh(t, stub, "jira", "--since", "2w", "--project", "rhdh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.pattern != rhdh.JiraKeyPattern {
		t.Errorf("pattern = %q, want default Jira pattern", stub.pattern)
	}
	if stub.opts.Since != "2w" || stub.opts.Project != "rhdh" {
		t.Errorf("options not forwarded: %+v", stub.opts)
	}
	for _, want := range []string{"sess-1", "RHIDP-1234", "claude-code"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRhdhJiraSpecificKeyJSON(t *testing.T) {
	stub := &stubSearcher{result: &upstream.SearchResult{Matches: []upstream.ContentMatch{
		{SessionID: "sess-1", Timestamp: "t1", Snippet: "RHIDP-1234 here"},
		{SessionID: "sess-2", Timestamp: "t2", Snippet: "unrelated OTHER-9"},
	}}}
	out, err := runRhdh(t, stub, "jira", "rhidp-1234", "--json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := `\b(RHIDP-1234)\b`; stub.pattern != want {
		t.Errorf("pattern = %q, want %q", stub.pattern, want)
	}
	var sessions []rhdh.JiraSession
	if err := json.Unmarshal([]byte(out), &sessions); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if len(sessions) != 1 || sessions[0].SessionID != "sess-1" {
		t.Errorf("got %+v, want only sess-1", sessions)
	}
}

func TestRhdhJiraInvalidKey(t *testing.T) {
	if _, err := runRhdh(t, &stubSearcher{}, "jira", "notakey"); err == nil {
		t.Fatal("expected error for invalid issue key")
	}
}

func TestRhdhPRJSONOutput(t *testing.T) {
	stub := &stubSearcher{result: &upstream.SearchResult{Matches: []upstream.ContentMatch{
		{SessionID: "sess-1", Project: "rhdh", Agent: "claude-code",
			Timestamp: "2026-07-01T10:00:00Z",
			Snippet:   "gh pr create then https://github.com/redhat-developer/rhdh/pull/42"},
	}}}
	out, err := runRhdh(t, stub, "pr", "--json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sessions []rhdh.PRSession
	if err := json.Unmarshal([]byte(out), &sessions); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if len(sessions) != 1 || !sessions[0].Created {
		t.Fatalf("got %+v, want one created session", sessions)
	}
	if got := sessions[0].PRs[0].URL(); got != "https://github.com/redhat-developer/rhdh/pull/42" {
		t.Errorf("PR URL = %q", got)
	}
}

func TestRhdhPRRepoFilter(t *testing.T) {
	stub := &stubSearcher{result: &upstream.SearchResult{Matches: []upstream.ContentMatch{
		{SessionID: "sess-1", Timestamp: "t1",
			Snippet: "https://github.com/redhat-developer/rhdh/pull/42"},
		{SessionID: "sess-2", Timestamp: "t2",
			Snippet: "https://github.com/other/repo/pull/1"},
	}}}
	out, err := runRhdh(t, stub, "pr", "42", "--repo", "redhat-developer/rhdh", "--json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sessions []rhdh.PRSession
	if err := json.Unmarshal([]byte(out), &sessions); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if len(sessions) != 1 || sessions[0].SessionID != "sess-1" {
		t.Errorf("got %+v, want only sess-1", sessions)
	}
}

func TestRhdhPREmptyResult(t *testing.T) {
	stub := &stubSearcher{result: &upstream.SearchResult{}}
	out, err := runRhdh(t, stub, "pr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No sessions") {
		t.Errorf("expected empty-result message, got:\n%s", out)
	}
}

func TestRootCommandExposesUpstreamCommands(t *testing.T) {
	root := NewRootCommand()
	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{
		"rhdh", "version",
		"serve", "daemon", "sync", "session", "usage", "stats", "export",
		"import", "mcp", "recall", "doctor", "openapi", "agentsview-version",
	} {
		if !names[want] {
			t.Errorf("root command missing subcommand %q", want)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), AgentsviewVersion) {
		t.Errorf("version output missing pinned upstream version: %s", out.String())
	}
}
