// Package upstream locates and drives the upstream `agentsview` binary
// (go.kenn.io/agentsview, pinned as a tool dependency in go.mod).
//
// AgentsView keeps all of its library code under internal/, so the CLI and
// its --json output are the supported integration surface. This package
// shells out to the binary and decodes its JSON.
package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// EnvBinary overrides the upstream binary location when set.
const EnvBinary = "AGENTSVIEW_BIN"

// InstallHint tells users how to obtain the upstream binary at the version
// this module is pinned to.
const InstallHint = "go install go.kenn.io/agentsview/cmd/agentsview@v0.39.0"

// Runner executes the upstream agentsview binary.
type Runner struct {
	// Binary is the resolved path or command name. Empty means unresolved.
	Binary string
}

// Resolve locates the agentsview binary: $AGENTSVIEW_BIN first, then PATH.
func Resolve() (*Runner, error) {
	if bin := os.Getenv(EnvBinary); bin != "" {
		return &Runner{Binary: bin}, nil
	}
	if bin, err := exec.LookPath("agentsview"); err == nil {
		return &Runner{Binary: bin}, nil
	}
	return nil, fmt.Errorf(
		"agentsview binary not found in PATH (set %s or run: %s)",
		EnvBinary, InstallHint)
}

// Exec runs the binary with args attached to the current stdio and returns
// its exit code. Used by the passthrough subcommands.
func (r *Runner) Exec(ctx context.Context, args ...string) int {
	cmd := exec.CommandContext(ctx, r.Binary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "agentsview: %v\n", err)
		return 1
	}
	return 0
}

// SearchOptions narrows a content search, mirroring the upstream
// `session search` flags this extension exposes.
type SearchOptions struct {
	Project string
	Agent   string
	Since   string
	Limit   int
}

// ContentMatch mirrors the JSON emitted by
// `agentsview session search --json` in v0.39.0.
type ContentMatch struct {
	SessionID string `json:"session_id"`
	Project   string `json:"project"`
	Agent     string `json:"agent"`
	Location  string `json:"location"` // message | tool_input | tool_result
	Role      string `json:"role"`
	ToolName  string `json:"tool_name,omitempty"`
	Ordinal   int    `json:"ordinal"`
	Timestamp string `json:"timestamp"`
	Snippet   string `json:"snippet"`
}

// SearchResult mirrors the top-level JSON object of
// `agentsview session search --json`.
type SearchResult struct {
	Matches    []ContentMatch `json:"matches"`
	NextCursor int            `json:"next_cursor,omitempty"`
}

// Searcher is the part of the upstream CLI the rhdh subcommands need;
// tests substitute a stub.
type Searcher interface {
	SearchContent(ctx context.Context, pattern string, opts SearchOptions) (*SearchResult, error)
}

// SearchContent runs `agentsview session search <pattern> --regex --json`,
// following the pagination cursor until exhausted or opts.Limit matches.
func (r *Runner) SearchContent(ctx context.Context, pattern string, opts SearchOptions) (*SearchResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 200
	}
	out := &SearchResult{}
	cursor := 0
	for len(out.Matches) < limit {
		args := []string{
			"session", "search", pattern,
			"--regex", "--json",
			"--limit", fmt.Sprint(limit - len(out.Matches)),
		}
		if cursor > 0 {
			args = append(args, "--cursor", fmt.Sprint(cursor))
		}
		if opts.Project != "" {
			args = append(args, "--project", opts.Project)
		}
		if opts.Agent != "" {
			args = append(args, "--agent", opts.Agent)
		}
		if opts.Since != "" {
			args = append(args, "--since", opts.Since)
		}
		page, err := r.searchPage(ctx, args)
		if err != nil {
			return nil, err
		}
		out.Matches = append(out.Matches, page.Matches...)
		if page.NextCursor == 0 {
			break
		}
		cursor = page.NextCursor
	}
	return out, nil
}

func (r *Runner) searchPage(ctx context.Context, args []string) (*SearchResult, error) {
	cmd := exec.CommandContext(ctx, r.Binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := bytes.TrimSpace(stderr.Bytes())
		if len(msg) > 0 {
			return nil, fmt.Errorf("agentsview session search: %s", msg)
		}
		return nil, fmt.Errorf("agentsview session search: %w", err)
	}
	res := &SearchResult{}
	if err := json.Unmarshal(stdout.Bytes(), res); err != nil {
		return nil, fmt.Errorf("decoding agentsview search output: %w", err)
	}
	return res, nil
}
