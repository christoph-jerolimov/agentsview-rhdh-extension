# agentsview-rhdh-extension

`agentsview-rhdh` is a Go CLI that wraps [AgentsView](https://github.com/kenn-io/agentsview)
(`go.kenn.io/agentsview`, pinned to **v0.39.0** via a `tool` directive in
`go.mod`) and adds RHDH-specific analysis of recorded AI coding agent
sessions:

- **Everything AgentsView provides** is exposed as passthrough subcommands
  (`serve`, `daemon`, `session`, `usage`, `stats`, `export`, `mcp`, …) that
  forward verbatim to the upstream binary.
- An **`rhdh` subcommand** analyses session content and finds:
  - sessions that **reference a Jira issue** (`rhdh jira`), and
  - sessions that **create or reference a GitHub pull request** (`rhdh pr`).

> AgentsView keeps all of its Go packages under `internal/`, so its CLI and
> `--json` output are the supported integration surface. This extension
> shells out to the pinned `agentsview` binary and post-processes its JSON.

## Install

```sh
# The upstream AgentsView binary (pinned version):
go install go.kenn.io/agentsview/cmd/agentsview@v0.39.0

# This CLI:
go build -o agentsview-rhdh .
```

The wrapper finds `agentsview` on your `PATH`; set `AGENTSVIEW_BIN` to point
at a specific binary instead.

## Usage

### AgentsView passthrough

Every upstream command works as usual, e.g.:

```sh
agentsview-rhdh serve
agentsview-rhdh session list --json
agentsview-rhdh usage daily
agentsview-rhdh session search "picocolors" --regex
agentsview-rhdh agentsview-version   # upstream `agentsview version`
```

### RHDH filters

Find sessions referencing Jira issues:

```sh
# Any Jira-shaped issue key (with a denylist for UTF-8, SHA-256, CVE-…, …):
agentsview-rhdh rhdh jira

# Specific issues only:
agentsview-rhdh rhdh jira RHIDP-1234 RHDHBUGS-42

# Scoped and machine-readable:
agentsview-rhdh rhdh jira RHIDP-1234 --project rhdh --since 2w --json
```

Find sessions that create or reference GitHub pull requests (PR URLs, plus
creation markers such as `gh pr create`, the GitHub MCP
`create_pull_request` tool, or "created a pull request"):

```sh
agentsview-rhdh rhdh pr
agentsview-rhdh rhdh pr --created-only
agentsview-rhdh rhdh pr 42 --repo redhat-developer/rhdh
agentsview-rhdh rhdh pr https://github.com/redhat-developer/rhdh/pull/42 --json
```

Both filters share `--project`, `--agent`, `--since`, `--limit`, and
`--json`, and group results per session (newest activity first) with the
issue keys / PRs found in each.

## Development

```sh
gofmt -l .
go vet ./...
go build -o agentsview-rhdh .
go test -race ./...
```

CI runs the same steps via [GitHub Actions](.github/workflows/ci.yml). Unit
tests stub the upstream binary with shell-script fakes, so they run without
a real AgentsView installation or session database.
