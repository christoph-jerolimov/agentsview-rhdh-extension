package upstream

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeFakeAgentsview installs a shell script standing in for the upstream
// binary and points $AGENTSVIEW_BIN at it.
func writeFakeAgentsview(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake binary helper uses a shell script")
	}
	path := filepath.Join(t.TempDir(), "agentsview")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvBinary, path)
	return path
}

func TestResolvePrefersEnvOverride(t *testing.T) {
	path := writeFakeAgentsview(t, "exit 0")
	r, err := Resolve()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Binary != path {
		t.Errorf("Binary = %q, want %q", r.Binary, path)
	}
}

func TestResolveMissingBinary(t *testing.T) {
	t.Setenv(EnvBinary, "")
	t.Setenv("PATH", t.TempDir())
	if _, err := Resolve(); err == nil {
		t.Fatal("expected error when binary is missing")
	}
}

func TestExecPropagatesExitCode(t *testing.T) {
	writeFakeAgentsview(t, "exit 3")
	r, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if code := r.Exec(context.Background(), "doctor"); code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}

func TestSearchContentDecodesAndPaginates(t *testing.T) {
	// Page 1 returns one match plus a cursor; page 2 (invoked with
	// --cursor 5) returns the final match.
	writeFakeAgentsview(t, `
for arg in "$@"; do
  if [ "$arg" = "--cursor" ]; then
    echo '{"matches":[{"session_id":"s2","project":"p","agent":"a","snippet":"two"}]}'
    exit 0
  fi
done
echo '{"matches":[{"session_id":"s1","project":"p","agent":"a","snippet":"one"}],"next_cursor":5}'
`)
	r, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.SearchContent(context.Background(), "pattern", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Matches) != 2 {
		t.Fatalf("got %d matches, want 2: %+v", len(res.Matches), res.Matches)
	}
	if res.Matches[0].SessionID != "s1" || res.Matches[1].SessionID != "s2" {
		t.Errorf("wrong pages: %+v", res.Matches)
	}
}

func TestSearchContentForwardsFilters(t *testing.T) {
	writeFakeAgentsview(t, `
echo "$@" >&2
case "$*" in
*"--project rhdh"*"--agent claude-code"*"--since 2w"*)
  echo '{"matches":[]}' ;;
*)
  echo "missing expected flags: $*" >&2; exit 1 ;;
esac
`)
	r, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.SearchContent(context.Background(), "p", SearchOptions{
		Project: "rhdh", Agent: "claude-code", Since: "2w",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearchContentReportsStderr(t *testing.T) {
	writeFakeAgentsview(t, `echo "database locked" >&2; exit 1`)
	r, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.SearchContent(context.Background(), "p", SearchOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "agentsview session search: database locked" {
		t.Errorf("error = %q", got)
	}
}
