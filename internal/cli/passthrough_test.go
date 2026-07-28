package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/christoph-jerolimov/agentsview-rhdh-extension/internal/upstream"
)

func TestPassthroughForwardsArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake binary helper uses a shell script")
	}
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	script := "#!/bin/sh\necho \"$@\" > " + argsFile + "\nexit 0\n"
	bin := filepath.Join(dir, "agentsview")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(upstream.EnvBinary, bin)

	// Flags after the passthrough command must reach the upstream binary
	// untouched (DisableFlagParsing).
	if code := Main([]string{"session", "list", "--json", "--limit", "5"}); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if want := "session list --json --limit 5"; strings.TrimSpace(string(got)) != want {
		t.Errorf("forwarded args = %q, want %q", strings.TrimSpace(string(got)), want)
	}
}

func TestPassthroughVersionRenamed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake binary helper uses a shell script")
	}
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	script := "#!/bin/sh\necho \"$@\" > " + argsFile + "\nexit 0\n"
	bin := filepath.Join(dir, "agentsview")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(upstream.EnvBinary, bin)

	if code := Main([]string{"agentsview-version"}); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if want := "version"; strings.TrimSpace(string(got)) != want {
		t.Errorf("forwarded args = %q, want %q", strings.TrimSpace(string(got)), want)
	}
}

func TestPassthroughPropagatesExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake binary helper uses a shell script")
	}
	bin := filepath.Join(t.TempDir(), "agentsview")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 4\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(upstream.EnvBinary, bin)

	if code := Main([]string{"doctor"}); code != 4 {
		t.Errorf("exit code = %d, want 4", code)
	}
}

func TestPassthroughMissingBinaryFails(t *testing.T) {
	t.Setenv(upstream.EnvBinary, "")
	t.Setenv("PATH", t.TempDir())
	if code := Main([]string{"serve"}); code == 0 {
		t.Error("expected non-zero exit code when agentsview binary is missing")
	}
}
