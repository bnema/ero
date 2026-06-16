# Plugins

Ero plugins are a general extension mechanism based on local subprocesses. A plugin declares one or more contributions in its manifest; future contribution types can extend other parts of Ero, such as themes or additional workflows.

The `review_provider` contribution type lets plugins publish reviews, load remote review threads, and provide provider-specific review context such as pull request metadata.

## Install and manage plugins

User-installed plugins are tracked in the XDG config directory and stored under the XDG data directory:

- config: `$XDG_CONFIG_HOME/ero/config.toml`, or `~/.config/ero/config.toml`
- data: `$XDG_DATA_HOME/ero`, or `~/.local/share/ero`
- cache helper path: `$XDG_CACHE_HOME/ero`, or `~/.cache/ero`

Commands:

```bash
ero plugin install <source>
ero plugin list
ero plugin update [source]
ero plugin remove <name|source>
```

All plugin subcommands support `--json` for machine-readable output. Sources may be Git URLs, `git:` shorthand such as `git:github.com/owner/repo@v1.2.3`, or a local Git repository path. Local repositories are registered by reference and are not deleted by Ero when removed.

### Shipped plugins

Ero ships maintained provider plugins under `plugins/`:

- `plugins/codex`
- `plugins/github`
- `plugins/pi-coding-agent`

These shipped plugins are available in provider discovery and the provider
picker without any `ero plugin install` step. `ero plugin list` renders them
with their manifest path and marks them as `managed shipped`, so they appear
consistently alongside user-installed providers while still being protected from
plugin lifecycle operations.

Packaged releases include shipped plugin assets next to the `ero` binary. Source
checkouts expose the same plugins when you run Ero from inside the repository
tree, where default shipped discovery can resolve the repo-local `plugins/`
directory. A bare `go install ./cmd/ero` or standalone `go build ./cmd/ero`
binary does not include plugin assets by itself.

Shipped plugins run through the same plugin/provider activation path as other
review providers: Ero discovers each `review_provider` contribution from its
`ero-plugin.toml`, selects it from the provider catalog, and starts it using the
normal plugin protocol.

Shipped plugins differ from user-installed plugins in these ways:

- **Always available in supported layouts**: shipped plugins appear in provider
  discovery and the provider picker in packaged releases and source-checkout
  runs, regardless of whether any user-installed plugins are installed.
- **Visible in lifecycle discovery**: `ero plugin list` includes shipped plugins
  and marks them as `managed shipped`.
- **Not installable/removable/updateable**: shipped plugins are included with
  Ero, are not written to plugin config, and are not managed like
  user-installed plugins. `ero plugin install codex`,
  `ero plugin install ero-plugin-codex`, and
  `ero plugin install bundled:ero-plugin-codex` report that the shipped plugin
  cannot be installed through the plugin lifecycle. The same applies to
  `ero plugin update ...` and `ero plugin remove ...` for shipped plugin
  identifiers.

User-installed plugins are still tracked in the Ero config file. Removing or
updating plugins only changes user-installed plugin entries; shipped plugins
remain available.

## Manifest

Each plugin repository has an `ero-plugin.toml` at its root:

```toml
name = "ero-plugin-example"
version = "0.1.0"
description = "Example Ero review provider"
manifest_version = "1"
protocol = "ero.plugin.v1"

[runtime]
command = "go run ./cmd/ero-plugin-example"

[build]
command = "go build ./cmd/ero-plugin-example"

[[contributions]]
type = "review_provider"
id = "example"
label = "Example"
```

Required fields are `name`, `version`, `manifest_version = "1"`, `protocol = "ero.plugin.v1"`, `runtime.command`, and at least one contribution with `type` and `id`. Contribution type strings are lower snake_case; the currently implemented public contribution type is `review_provider`.

Ero discovers available review providers from shipped and user-installed plugin manifests before starting plugin subprocesses. Each discovered provider has a host-owned stable key derived from the plugin identity plus the contribution `id`; runtime provider IDs returned by `initialize` remain provider-owned metadata and are not used as the host selection key.

Ero keeps the plugin system global while activating only one review provider at a time. The TUI can switch providers, manually refresh the active provider, show cache/sync status, and display provider overview data in the PR sheet. Inactive providers remain descriptors plus cached/previously observed status; Ero does not start inactive provider subprocesses just to populate the picker.

Provider snapshots are normalized Ero data stored under the XDG cache directory. Ero loads cached provider data first, refreshes in the background, and keeps good cached data when refresh fails.

`runtime.command` is executed with the plugin root as the working directory. Keep it stable for installed users; use the optional `build.command` for local development or release packaging.

Shipped plugins use this split explicitly:

- Packaged releases ship both `ero` and the `plugins/` package, so default
  discovery resolves shipped manifests and runtimes via executable-relative
  paths.
- Source checkouts discover the same packages through the repo-local `plugins/`
  tree when you run Ero from inside the checkout.
- A bare `go install ./cmd/ero` or `go build ./cmd/ero` binary does not include
  those plugin assets; use the packaged layout when you want shipped providers
  outside the repo tree.

For Codex, `runtime.command = "./bin/ero-plugin-codex"` is the packaged runtime
layout. Its `build.command` lets source checkouts rebuild the same runtime path
when local plugin source is present.
- Packaged releases ship the ready-to-run binary at that path and do not rely on
  source files being present at runtime.

## Protocol

Ero communicates with plugins over newline-delimited JSON on stdin/stdout. Each stdout line must be exactly one JSON response envelope. Diagnostics, logs, and dry-run output must go to stderr so stdout remains parseable.

Request envelope:

```json
{"id":"1","method":"initialize","params":{"protocol":"ero.plugin.v1","contribution_id":"example"}}
```

`contribution_id` is the `id` from the selected manifest contribution. Ero starts one review-provider client per `review_provider` contribution and passes that ID during initialization, so a single plugin package can expose multiple review providers when its runtime routes by contribution ID.

Response envelope:

```json
{"id":"1","result":{"protocol":"ero.plugin.v1","provider":{}}}
```

Errors use structured codes:

```json
{"id":"1","error":{"code":"auth_required","message":"set a token"}}
```

Review provider methods:

- `initialize`: negotiate `ero.plugin.v1`, bind to the requested `contribution_id`, and return provider metadata/capabilities.
- `detect_context`: decide whether the current repository/review context applies.
- `load_remote_threads`: return remote review threads when `load_remote_comments` is supported.
- `load_remote_snapshot`: return remote review threads plus provider overview data when `load_remote_snapshot` is supported. Hosts prefer this method when advertised and fall back to `load_remote_threads` for older providers.
- `publish_review`: publish a draft review when `publish_review` is supported.

Capabilities include `load_remote_comments`, `load_remote_snapshot`, `publish_review`, supported `decisions` (`comment`, `request_changes`, `approve`), and `idempotent_publish`.

## Go SDK

Plugins can implement the protocol with `pkg/plugin`:

```go
package main

import (
    "context"
    "fmt"
    "os"

    "ero/pkg/plugin"
)

func main() {
    provider := myProvider{}
    if err := plugin.ServeReviewProvider(context.Background(), provider, os.Stdin, os.Stdout); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

Implement `plugin.ReviewProvider` and return `plugin.NewError(...)` for structured failures such as `plugin.ErrorAuthRequired`, `plugin.ErrorNotApplicable`, or `plugin.ErrorUnsupportedCapability`.

## Secrets policy

Do not put secrets in `ero-plugin.toml`, command-line arguments, or stdout. Read credentials from environment variables or the platform credential store. Plugins should return `auth_required` when credentials are missing and should avoid logging tokens or request payloads containing secrets.

## Maintained plugins

The `plugins/` directory contains maintained plugin implementations:

- `plugins/codex`: shipped Codex review provider (callback-only). Its manifest declares `ero-plugin-codex` with the `codex` review-provider contribution, its packaged runtime lives at `plugins/codex/bin/ero-plugin-codex`, and source checkouts can rebuild that path through the manifest `build.command`.

  The Codex provider publishes reviews via callback into an explicit Codex session. It requires two environment variables:
  - `ERO_CODEX_SOCKET_PATH` — path to the Codex app-server control socket
  - `ERO_CODEX_THREAD_ID` — the target thread/session to publish into

  This is strictly callback-only: the provider does **not** auto-select a thread by CWD, scan stored threads for matches, create new threads, fall back to stdio subprocess mode, or detect a Codex binary. If either required variable is missing, `detect_context` returns not applicable. Publishing connects to the running Codex app-server over its Unix control socket, initializes a JSON-RPC handshake, and sends the formatted review message as a user turn on the configured thread (`turn/start`).
- `plugins/github`: GitHub review provider. It is a maintained installable plugin, not a shipped provider. It uses GitHub CLI-compatible authentication through `go-gh`, so `gh auth login` must be configured. The provider parses GitHub remotes, detects the matching pull request for the current branch/range context, fetches PR metadata, issue comments, review summaries, and review threads through GraphQL, and publishes reviews to the matched pull request. Publishing returns a fast error when no matching pull request is available.
- `plugins/pi-coding-agent`: pi-coding-agent destination. It is a maintained installable plugin, not a shipped provider. Load its Pi extension, then Ero can publish a review into the matching Pi session as a user message.

Build the maintained plugins with:

```bash
go test ./plugins/...
go build ./plugins/codex/cmd/ero-plugin-codex ./plugins/github/cmd/ero-plugin-github ./plugins/pi-coding-agent/cmd/ero-plugin-pi-coding-agent
```

For pi-coding-agent, install the bridge extension package first:

```bash
pi install ./plugins/pi-coding-agent
```

For a one-off development session, `pi -e ./plugins/pi-coding-agent` also works, but it only loads the extension for that run.

The bridge records active sessions in an owner-only runtime registry and uses per-session Unix sockets. Ero selects a session by `PI_CODING_AGENT_SESSION_ID` when set, otherwise by repository path plus branch/SHA when available.

## Current limitations

Ero review providers run as local subprocesses. Ero does not provide a sandbox, plugin marketplace, background daemon, automatic secret storage, or full forge implementations. Remote APIs, authentication flows, and provider-specific publish semantics belong in individual plugins.
