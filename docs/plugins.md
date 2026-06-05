# Plugins

Ero plugins are a general extension mechanism based on local subprocesses. A plugin declares one or more contributions in its manifest; future contribution types can extend other parts of Ero, such as themes or additional workflows.

This first release ships the `review_provider` contribution type, which lets plugins publish reviews and load remote review comments.

## Install and manage plugins

Plugins are tracked in the XDG config directory and stored under the XDG data directory:

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

Ero discovers available review providers from installed plugin manifests before starting plugin subprocesses. Each discovered provider has a host-owned stable key derived from a canonical installed-plugin identity plus the contribution `id`; runtime provider IDs returned by `initialize` remain provider-owned metadata and are not used as the host selection key.

`runtime.command` is executed with the plugin root as the working directory. Keep it stable for installed users; use the optional `build.command` for local development or release packaging.

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
- `load_remote_threads`: return remote review comments when `load_remote_comments` is supported.
- `publish_review`: publish a draft review when `publish_review` is supported.

Capabilities include `load_remote_comments`, `publish_review`, supported `decisions` (`comment`, `request_changes`, `approve`), and `idempotent_publish`.

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

## Maintained default plugins

Ero ships maintained plugin implementations under `plugins/`:

- `plugins/github`: GitHub review provider. It requires the GitHub CLI (`gh`) installed and authenticated with `gh auth login`; the plugin uses `go-gh`/`gh` for GitHub auth, current-branch PR lookup, and PR review submission. Publishing returns a fast error when the current branch has no associated pull request.
- `plugins/pi-coding-agent`: pi-coding-agent destination. Load its Pi extension, then Ero can publish a review into the matching Pi session as a user message.

Build them with:

```bash
go test ./plugins/...
go build ./plugins/github/cmd/ero-plugin-github ./plugins/pi-coding-agent/cmd/ero-plugin-pi-coding-agent
```

For pi-coding-agent, install the bridge extension package first:

```bash
pi install ./plugins/pi-coding-agent
```

For a one-off development session, `pi -e ./plugins/pi-coding-agent` also works, but it only loads the extension for that run.

The bridge records active sessions in an owner-only runtime registry and uses per-session Unix sockets. Ero selects a session by `PI_CODING_AGENT_SESSION_ID` when set, otherwise by repository path plus branch/SHA when available.

## First-release limitations

The first plugin release focuses on review providers launched as local subprocesses. Ero does not provide a sandbox, plugin marketplace, background daemon, automatic secret storage, or full forge implementations. Remote APIs, authentication flows, and provider-specific publish semantics belong in individual plugins.
