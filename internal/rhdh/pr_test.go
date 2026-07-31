package rhdh

import (
	"reflect"
	"testing"

	"github.com/christoph-jerolimov/agentsview-rhdh-extension/internal/upstream"
)

func TestParsePRFilter(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		repo    string
		want    PRFilter
		wantErr bool
	}{
		{name: "empty", want: PRFilter{}},
		{name: "number", arg: "42", want: PRFilter{Number: 42}},
		{name: "hash number", arg: "#42", want: PRFilter{Number: 42}},
		{name: "repo flag", repo: "redhat-developer/rhdh",
			want: PRFilter{Owner: "redhat-developer", Repo: "rhdh"}},
		{name: "url", arg: "https://github.com/redhat-developer/rhdh/pull/42",
			want: PRFilter{Owner: "redhat-developer", Repo: "rhdh", Number: 42}},
		{name: "number plus repo", arg: "7", repo: "a/b",
			want: PRFilter{Owner: "a", Repo: "b", Number: 7}},
		{name: "bad repo", repo: "no-slash", wantErr: true},
		{name: "bad arg", arg: "nonsense", wantErr: true},
		{name: "negative number", arg: "-3", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePRFilter(tt.arg, tt.repo)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestExtractPRRefs(t *testing.T) {
	snippet := "Opened https://github.com/redhat-developer/rhdh/pull/42 and " +
		"referenced github.com/janus-idp/backstage-plugins/pull/7 twice: " +
		"https://github.com/janus-idp/backstage-plugins/pull/7"
	got := ExtractPRRefs(snippet, PRFilter{})
	want := []PRRef{
		{Owner: "redhat-developer", Repo: "rhdh", Number: 42},
		{Owner: "janus-idp", Repo: "backstage-plugins", Number: 7},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	filtered := ExtractPRRefs(snippet, PRFilter{Owner: "redhat-developer", Repo: "rhdh"})
	if len(filtered) != 1 || filtered[0].Number != 42 {
		t.Errorf("repo filter: got %+v, want only rhdh#42", filtered)
	}

	byNumber := ExtractPRRefs(snippet, PRFilter{Number: 7})
	if len(byNumber) != 1 || byNumber[0].Repo != "backstage-plugins" {
		t.Errorf("number filter: got %+v, want only #7", byNumber)
	}
}

func TestHasPRCreationMarker(t *testing.T) {
	for _, s := range []string{
		"ran `gh pr create --fill`",
		"calling mcp__github__create_pull_request",
		"I created a pull request for the fix",
		"Created pull request #12",
	} {
		if !HasPRCreationMarker(s) {
			t.Errorf("expected creation marker in %q", s)
		}
	}
	for _, s := range []string{
		"reviewing the pull request",
		"gh pr view 42",
	} {
		if HasPRCreationMarker(s) {
			t.Errorf("unexpected creation marker in %q", s)
		}
	}
}

func TestGroupPRSessions(t *testing.T) {
	matches := []upstream.ContentMatch{
		{SessionID: "s1", Project: "rhdh", Agent: "claude-code",
			Timestamp: "2026-07-01T10:00:00Z",
			Snippet:   "gh pr create --title fix"},
		{SessionID: "s1", Project: "rhdh", Agent: "claude-code",
			Timestamp: "2026-07-01T11:00:00Z",
			Snippet:   "https://github.com/redhat-developer/rhdh/pull/42"},
		{SessionID: "s2", Project: "docs", Agent: "cursor",
			Timestamp: "2026-07-02T09:00:00Z",
			Snippet:   "see github.com/janus-idp/backstage-plugins/pull/7"},
	}

	got := GroupPRSessions(matches, PRFilter{}, false)
	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2: %+v", len(got), got)
	}
	if got[0].SessionID != "s2" || got[1].SessionID != "s1" {
		t.Errorf("wrong order: %+v", got)
	}
	s1 := got[1]
	if !s1.Created || s1.MatchCount != 2 {
		t.Errorf("s1: created=%v matches=%d, want created with 2 matches", s1.Created, s1.MatchCount)
	}
	if len(s1.PRs) != 1 || s1.PRs[0].String() != "redhat-developer/rhdh#42" {
		t.Errorf("s1 PRs = %+v", s1.PRs)
	}
	if got[0].Created {
		t.Errorf("s2 should not be marked created")
	}
}

func TestGroupPRSessionsCreatedOnly(t *testing.T) {
	matches := []upstream.ContentMatch{
		{SessionID: "s1", Timestamp: "t1", Snippet: "gh pr create"},
		{SessionID: "s2", Timestamp: "t2",
			Snippet: "https://github.com/a/b/pull/1"},
	}
	got := GroupPRSessions(matches, PRFilter{}, true)
	if len(got) != 1 || got[0].SessionID != "s1" {
		t.Errorf("got %+v, want only s1", got)
	}
}

func TestGroupPRSessionsFilterKeepsCreatedFlag(t *testing.T) {
	// With a repo filter, a creation-marker-only match does not qualify a
	// session by itself, but it must still mark a qualifying session (one
	// with a matching URL) as created.
	matches := []upstream.ContentMatch{
		{SessionID: "s1", Timestamp: "t1", Snippet: "gh pr create"},
		{SessionID: "s1", Timestamp: "t2",
			Snippet: "https://github.com/a/b/pull/1"},
		{SessionID: "s2", Timestamp: "t3", Snippet: "gh pr create"},
	}
	got := GroupPRSessions(matches, PRFilter{Owner: "a", Repo: "b"}, false)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1: %+v", len(got), got)
	}
	if got[0].SessionID != "s1" || !got[0].Created {
		t.Errorf("got %+v, want s1 marked created", got[0])
	}
}
