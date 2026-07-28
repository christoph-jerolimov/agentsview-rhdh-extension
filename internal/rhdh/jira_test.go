package rhdh

import (
	"reflect"
	"testing"

	"github.com/christoph-jerolimov/agentsview-rhdh-extension/internal/upstream"
)

func TestNormalizeJiraKeys(t *testing.T) {
	keys, err := NormalizeJiraKeys([]string{"rhidp-1234", "RHDHBUGS-42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"RHIDP-1234", "RHDHBUGS-42"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("got %v, want %v", keys, want)
	}

	for _, bad := range []string{"1234", "RHIDP", "RHIDP-", "-1234", "not a key"} {
		if _, err := NormalizeJiraKeys([]string{bad}); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestJiraSearchPattern(t *testing.T) {
	if got := JiraSearchPattern(nil); got != JiraKeyPattern {
		t.Errorf("empty keys: got %q, want %q", got, JiraKeyPattern)
	}
	got := JiraSearchPattern([]string{"RHIDP-1234", "RHDHBUGS-42"})
	want := `\b(RHIDP-1234|RHDHBUGS-42)\b`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractJiraKeys(t *testing.T) {
	tests := []struct {
		name    string
		snippet string
		wanted  []string
		want    []string
	}{
		{
			name:    "finds and dedupes keys",
			snippet: "Fixes RHIDP-1234 and RHIDP-1234, relates to RHDHBUGS-42",
			want:    []string{"RHDHBUGS-42", "RHIDP-1234"},
		},
		{
			name:    "denylist filters technical tokens",
			snippet: "UTF-8 ISO-8601 SHA-256 RFC-7231 CVE-2024-1234 but RHIDP-99 is real",
			want:    []string{"RHIDP-99"},
		},
		{
			name:    "wanted filter keeps only requested keys",
			snippet: "RHIDP-1 RHIDP-2 OTHER-3",
			wanted:  []string{"RHIDP-2"},
			want:    []string{"RHIDP-2"},
		},
		{
			name:    "lowercase keys do not match",
			snippet: "rhidp-1234 is not a key reference",
			want:    nil,
		},
		{
			name:    "no keys",
			snippet: "just some text",
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractJiraKeys(tt.snippet, tt.wanted)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGroupJiraSessions(t *testing.T) {
	matches := []upstream.ContentMatch{
		{SessionID: "s1", Project: "rhdh", Agent: "claude-code",
			Timestamp: "2026-07-01T10:00:00Z", Snippet: "working on RHIDP-1234"},
		{SessionID: "s1", Project: "rhdh", Agent: "claude-code",
			Timestamp: "2026-07-01T12:00:00Z", Snippet: "RHDHBUGS-42 also RHIDP-1234"},
		{SessionID: "s2", Project: "other", Agent: "cursor",
			Timestamp: "2026-07-02T09:00:00Z", Snippet: "see RHIDP-7"},
		{SessionID: "s3", Project: "noise", Agent: "cursor",
			Timestamp: "2026-07-03T09:00:00Z", Snippet: "UTF-8 only"},
	}
	got := GroupJiraSessions(matches, nil)
	want := []JiraSession{
		{SessionID: "s2", Project: "other", Agent: "cursor",
			FirstSeen: "2026-07-02T09:00:00Z", LastSeen: "2026-07-02T09:00:00Z",
			MatchCount: 1, IssueKeys: []string{"RHIDP-7"}},
		{SessionID: "s1", Project: "rhdh", Agent: "claude-code",
			FirstSeen: "2026-07-01T10:00:00Z", LastSeen: "2026-07-01T12:00:00Z",
			MatchCount: 2, IssueKeys: []string{"RHDHBUGS-42", "RHIDP-1234"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v,\nwant %+v", got, want)
	}
}

func TestGroupJiraSessionsWantedFilter(t *testing.T) {
	matches := []upstream.ContentMatch{
		{SessionID: "s1", Timestamp: "t1", Snippet: "RHIDP-1"},
		{SessionID: "s2", Timestamp: "t2", Snippet: "OTHER-2"},
	}
	got := GroupJiraSessions(matches, []string{"RHIDP-1"})
	if len(got) != 1 || got[0].SessionID != "s1" {
		t.Errorf("got %+v, want only s1", got)
	}
}
