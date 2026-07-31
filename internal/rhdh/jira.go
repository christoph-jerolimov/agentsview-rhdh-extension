// Package rhdh analyses AgentsView session content for RHDH workflows:
// finding sessions that reference Jira issues or create/reference GitHub
// pull requests.
package rhdh

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/christoph-jerolimov/agentsview-rhdh-extension/internal/upstream"
)

// JiraKeyPattern is the RE2 pattern sent to `agentsview session search
// --regex` to find Jira issue keys (e.g. RHIDP-1234) in message and tool
// content.
const JiraKeyPattern = `\b[A-Z][A-Z0-9]{1,9}-[0-9]{1,7}\b`

var jiraKeyRe = regexp.MustCompile(JiraKeyPattern)

// jiraDenylist holds uppercase prefixes that match the Jira key shape but
// are common technical tokens, not issue keys.
var jiraDenylist = map[string]bool{
	"UTF":   true, // UTF-8
	"ISO":   true, // ISO-8601
	"SHA":   true, // SHA-256
	"RFC":   true, // RFC-7231
	"AES":   true, // AES-256
	"CVE":   true, // CVE-2024-1234
	"HTTP":  true, // HTTP-2
	"OAUTH": true,
}

// jiraKeyShapeRe validates a user-supplied issue key argument.
var jiraKeyShapeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{1,9}-[0-9]{1,7}$`)

// NormalizeJiraKeys validates and uppercases user-supplied issue keys.
func NormalizeJiraKeys(args []string) ([]string, error) {
	keys := make([]string, 0, len(args))
	for _, a := range args {
		if !jiraKeyShapeRe.MatchString(a) {
			return nil, fmt.Errorf("%q does not look like a Jira issue key (expected e.g. RHIDP-1234)", a)
		}
		keys = append(keys, strings.ToUpper(a))
	}
	return keys, nil
}

// JiraSearchPattern builds the RE2 pattern for the upstream search. With no
// keys it matches any Jira-shaped key; with keys it matches exactly those.
func JiraSearchPattern(keys []string) string {
	if len(keys) == 0 {
		return JiraKeyPattern
	}
	quoted := make([]string, len(keys))
	for i, k := range keys {
		quoted[i] = regexp.QuoteMeta(k)
	}
	return `\b(` + strings.Join(quoted, "|") + `)\b`
}

// ExtractJiraKeys returns the deduplicated, denylist-filtered Jira keys
// found in a snippet. When wanted is non-empty only those keys count.
func ExtractJiraKeys(snippet string, wanted []string) []string {
	wantedSet := map[string]bool{}
	for _, k := range wanted {
		wantedSet[k] = true
	}
	seen := map[string]bool{}
	var keys []string
	for _, key := range jiraKeyRe.FindAllString(snippet, -1) {
		prefix := key[:strings.IndexByte(key, '-')]
		if jiraDenylist[prefix] {
			continue
		}
		if len(wantedSet) > 0 && !wantedSet[key] {
			continue
		}
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// JiraSession is a session that references one or more Jira issues.
type JiraSession struct {
	SessionID  string   `json:"session_id"`
	Project    string   `json:"project"`
	Agent      string   `json:"agent"`
	FirstSeen  string   `json:"first_seen,omitempty"`
	LastSeen   string   `json:"last_seen,omitempty"`
	MatchCount int      `json:"match_count"`
	IssueKeys  []string `json:"issue_keys"`
}

// GroupJiraSessions folds content matches into per-session Jira results,
// newest activity first. Matches whose snippets yield no surviving keys
// (denylist, wanted filter) are dropped.
func GroupJiraSessions(matches []upstream.ContentMatch, wanted []string) []JiraSession {
	byID := map[string]*JiraSession{}
	keysByID := map[string]map[string]bool{}
	var order []string
	for _, m := range matches {
		keys := ExtractJiraKeys(m.Snippet, wanted)
		if len(keys) == 0 {
			continue
		}
		s, ok := byID[m.SessionID]
		if !ok {
			s = &JiraSession{
				SessionID: m.SessionID,
				Project:   m.Project,
				Agent:     m.Agent,
				FirstSeen: m.Timestamp,
				LastSeen:  m.Timestamp,
			}
			byID[m.SessionID] = s
			keysByID[m.SessionID] = map[string]bool{}
			order = append(order, m.SessionID)
		}
		s.MatchCount++
		updateSeen(&s.FirstSeen, &s.LastSeen, m.Timestamp)
		for _, k := range keys {
			keysByID[m.SessionID][k] = true
		}
	}
	out := make([]JiraSession, 0, len(order))
	for _, id := range order {
		s := byID[id]
		s.IssueKeys = sortedKeys(keysByID[id])
		out = append(out, *s)
	}
	sortByLastSeen(out, func(s JiraSession) string { return s.LastSeen })
	return out
}

func updateSeen(first, last *string, ts string) {
	if ts == "" {
		return
	}
	if *first == "" || ts < *first {
		*first = ts
	}
	if ts > *last {
		*last = ts
	}
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortByLastSeen[T any](items []T, lastSeen func(T) string) {
	sort.SliceStable(items, func(i, j int) bool {
		return lastSeen(items[i]) > lastSeen(items[j])
	})
}
