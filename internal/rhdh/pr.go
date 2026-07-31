package rhdh

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/christoph-jerolimov/agentsview-rhdh-extension/internal/upstream"
)

// prURLPattern matches GitHub pull request URLs (and bare
// owner/repo/pull/N references) in session content.
const prURLPattern = `github\.com/[\w.-]+/[\w.-]+/pull/[0-9]+`

// prCreatePattern matches markers of a PR being created from within a
// session: the gh CLI, the GitHub MCP tool, or the agent saying so.
const prCreatePattern = `gh pr create|create_pull_request|created (a )?pull request`

var (
	prURLRe    = regexp.MustCompile(`(?i)github\.com/([\w.-]+)/([\w.-]+)/pull/([0-9]+)`)
	prCreateRe = regexp.MustCompile(`(?i)\bgh pr create\b|create_pull_request|\bcreated (a )?pull request\b`)
)

// PRSearchPattern builds the RE2 pattern for the upstream search, matching
// PR URL references and PR creation markers.
func PRSearchPattern() string {
	return `(?i)(` + prURLPattern + `|` + prCreatePattern + `)`
}

// PRRef identifies a GitHub pull request.
type PRRef struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
}

func (r PRRef) String() string {
	return fmt.Sprintf("%s/%s#%d", r.Owner, r.Repo, r.Number)
}

// URL returns the canonical GitHub URL of the pull request.
func (r PRRef) URL() string {
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d", r.Owner, r.Repo, r.Number)
}

// PRFilter narrows PR results to a repository and/or PR number. Zero
// values match everything.
type PRFilter struct {
	Owner  string
	Repo   string
	Number int
}

// ParsePRFilter interprets an optional positional argument (a PR number or
// a full/partial GitHub PR URL) plus a --repo owner/name flag.
func ParsePRFilter(arg, repo string) (PRFilter, error) {
	var f PRFilter
	if repo != "" {
		owner, name, ok := strings.Cut(repo, "/")
		if !ok || owner == "" || name == "" {
			return f, fmt.Errorf("--repo must be owner/name, got %q", repo)
		}
		f.Owner, f.Repo = owner, name
	}
	if arg == "" {
		return f, nil
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(arg, "#")); err == nil {
		if n <= 0 {
			return f, fmt.Errorf("PR number must be positive, got %d", n)
		}
		f.Number = n
		return f, nil
	}
	m := prURLRe.FindStringSubmatch(arg)
	if m == nil {
		return f, fmt.Errorf("%q is neither a PR number nor a GitHub pull request URL", arg)
	}
	f.Owner, f.Repo = m[1], m[2]
	f.Number, _ = strconv.Atoi(m[3])
	return f, nil
}

func (f PRFilter) matches(r PRRef) bool {
	if f.Owner != "" && !strings.EqualFold(f.Owner, r.Owner) {
		return false
	}
	if f.Repo != "" && !strings.EqualFold(f.Repo, r.Repo) {
		return false
	}
	if f.Number != 0 && f.Number != r.Number {
		return false
	}
	return true
}

// ExtractPRRefs returns the deduplicated PR references in a snippet that
// pass the filter.
func ExtractPRRefs(snippet string, filter PRFilter) []PRRef {
	seen := map[string]bool{}
	var refs []PRRef
	for _, m := range prURLRe.FindAllStringSubmatch(snippet, -1) {
		n, _ := strconv.Atoi(m[3])
		ref := PRRef{Owner: m[1], Repo: m[2], Number: n}
		if !filter.matches(ref) {
			continue
		}
		key := strings.ToLower(ref.String())
		if !seen[key] {
			seen[key] = true
			refs = append(refs, ref)
		}
	}
	return refs
}

// HasPRCreationMarker reports whether a snippet indicates a pull request
// was created in the session.
func HasPRCreationMarker(snippet string) bool {
	return prCreateRe.MatchString(snippet)
}

// PRSession is a session that created and/or referenced GitHub pull
// requests.
type PRSession struct {
	SessionID  string  `json:"session_id"`
	Project    string  `json:"project"`
	Agent      string  `json:"agent"`
	FirstSeen  string  `json:"first_seen,omitempty"`
	LastSeen   string  `json:"last_seen,omitempty"`
	MatchCount int     `json:"match_count"`
	Created    bool    `json:"created"`
	PRs        []PRRef `json:"prs"`
}

// GroupPRSessions folds content matches into per-session PR results,
// newest activity first.
//
// A session qualifies when it references a PR passing the filter, or —
// when no repo/number filter is set — when it merely carries a creation
// marker. With createdOnly, only sessions with a creation marker survive.
func GroupPRSessions(matches []upstream.ContentMatch, filter PRFilter, createdOnly bool) []PRSession {
	filtered := filter != (PRFilter{})
	createdByID := map[string]bool{}
	for _, m := range matches {
		if HasPRCreationMarker(m.Snippet) {
			createdByID[m.SessionID] = true
		}
	}
	byID := map[string]*PRSession{}
	refsByID := map[string]map[string]PRRef{}
	var order []string
	for _, m := range matches {
		refs := ExtractPRRefs(m.Snippet, filter)
		created := HasPRCreationMarker(m.Snippet)
		if len(refs) == 0 && !(created && !filtered) {
			continue
		}
		s, ok := byID[m.SessionID]
		if !ok {
			s = &PRSession{
				SessionID: m.SessionID,
				Project:   m.Project,
				Agent:     m.Agent,
				FirstSeen: m.Timestamp,
				LastSeen:  m.Timestamp,
			}
			byID[m.SessionID] = s
			refsByID[m.SessionID] = map[string]PRRef{}
			order = append(order, m.SessionID)
		}
		s.MatchCount++
		s.Created = s.Created || created
		updateSeen(&s.FirstSeen, &s.LastSeen, m.Timestamp)
		for _, r := range refs {
			refsByID[m.SessionID][strings.ToLower(r.String())] = r
		}
	}
	out := make([]PRSession, 0, len(order))
	for _, id := range order {
		s := byID[id]
		s.Created = s.Created || createdByID[id]
		if createdOnly && !s.Created {
			continue
		}
		s.PRs = sortedRefs(refsByID[id])
		out = append(out, *s)
	}
	sortByLastSeen(out, func(s PRSession) string { return s.LastSeen })
	return out
}

func sortedRefs(set map[string]PRRef) []PRRef {
	refs := make([]PRRef, 0, len(set))
	for _, r := range set {
		refs = append(refs, r)
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Owner != refs[j].Owner {
			return refs[i].Owner < refs[j].Owner
		}
		if refs[i].Repo != refs[j].Repo {
			return refs[i].Repo < refs[j].Repo
		}
		return refs[i].Number < refs[j].Number
	})
	return refs
}
