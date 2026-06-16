# ero

A terminal UI for reviewing Git diffs file by file.

`ero` opens the current repository in a GitHub-style review flow, with syntax-highlighted diffs, file navigation, search, and expandable context.

## Features

- smart startup mode for local changes, staged changes, upstream commits, or branch diffs
- explicit review modes for branch, working tree, staged, local, upstream, commit, and range diffs
- per-file navigation with a floating file finder
- reference search across the loaded review
- expandable unchanged context around diff hunks
- syntax-aware diff rendering
- selection copy support
- active review-provider sync with inline remote review threads
- GitHub PR overview sheet with Markdown-rendered PR body, issue comments, and review summaries

## Install

For the full bundled-Codex experience, use a packaged release from the GitHub releases page.

For source-checkout development, you can install the CLI with:

```bash
go install ./cmd/ero
```

`go install ./cmd/ero` installs only the `ero` binary. Bundled Codex assets are included in packaged release layouts, and source checkouts expose `plugins/codex` when you run Ero from inside the repository tree. A standalone `go install` or `go build` binary copied elsewhere does not carry bundled plugin assets by itself.

## Usage

```bash
ero                  # choose a safe review scope automatically
ero branch           # current branch vs default branch
ero working          # unstaged and untracked changes
ero staged           # staged changes
ero local            # staged + unstaged + untracked changes
ero upstream [ref]   # changes against upstream, default @{upstream}
ero commit <rev>     # one commit
ero range <base> <head>
```

Useful flags:

```bash
ero --repo-path /path/to/repo
ero --context-lines 5
```

## Plugins

Ero supports a general local subprocess plugin system, managed with `ero plugin install`, `ero plugin list`, `ero plugin update`, and `ero plugin remove`. Codex ships as a bundled/default plugin in packaged releases and source checkouts run from inside the repo tree: it is available without installation, appears in provider discovery and `ero plugin list`, runs through the normal plugin/provider path, and is not removable or updateable like user-installed plugins. A plain `go install ./cmd/ero` binary does not include bundled plugin assets unless you also package `plugins/codex` beside it. The first public contribution type is `review_provider`, used by Codex and by the maintained GitHub and pi-coding-agent plugins.

Ero discovers all provider contributions but activates one review provider at a time. The TUI supports provider switching, manual refresh, cache-first sync, and provider sync status. The GitHub plugin uses GitHub CLI-compatible authentication through `go-gh` and requires `gh auth login`. See [docs/plugins.md](docs/plugins.md) for authoring details.

## Development

```bash
make test
make fmt
make tidy
```
