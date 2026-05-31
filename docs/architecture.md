# Ero architecture

## Principles

- hexagonal architecture first
- `core` owns domain entities, value objects, and core business/application services
- `ports` live alongside `core`, not inside it
- there is no separate `usecase` package in this project shape
- `app` is the composition root and dependency injection point
- adapters translate CLI, TUI, git, syntax, and future agent integrations
- `cmd/ero/main.go` stays minimal
- the Bubble Tea root model stays thin
- reusable TUI components own rendering and local interaction details
- no ANSI strings or Lip Gloss styles in the core

## v1 UX target

The viewer is not a raw patch browser.

It should feel closer to a GitHub review:

- navigate changed files one by one
- render each file as ordered review sections
- show changed blocks with a small amount of surrounding context
- collapse large unchanged regions into explicit context-expander rows
- support expand above, expand below, and expand all around a section

That expansion behavior is domain business logic, so it belongs in the core review model, not in ad hoc TUI code.

## Planned layout

```text
cmd/ero/main.go                  minimal process entrypoint only

internal/app/                       composition root and dependency injection point
internal/core/                      domain entities and core business/application services
internal/ports/                     boundary interfaces used by core/adapters

internal/adapters/in/cli/           Cobra + Viper commands and flag binding
internal/adapters/in/tui/           Bubble Tea app shell
internal/adapters/in/tui/component/ reusable TUI components
internal/adapters/in/tui/presenter/ core state -> render models
internal/adapters/in/tui/render/    Lip Gloss rendering helpers
internal/adapters/in/tui/theme/     palettes and style factories
internal/adapters/in/tui/keymap/    user actions and key descriptions

internal/adapters/out/git/          go-git-backed branch, ref, status, and diff loading
internal/adapters/out/syntax/chroma/
                                    syntax tokenization adapter

test/                               integration fixtures and golden data
```

## Responsibilities by layer

### `cmd/ero/main.go`

Only:

- call the `app` package entrypoint
- map returned errors to exit codes

No business logic. No dependency wiring. No Bubble Tea setup here.

### `internal/app/`

This package is the composition root and dependency injection point.

It composes:

- go-git-backed repository adapter
- syntax adapter
- core business/application services
- startup mode detection and mixed-change prompting
- CLI adapter
- TUI adapter factories

It should expose the minimal runtime entrypoints needed by `main`, such as `Run()` or a root command/program constructor.

### `internal/core/`

Owns the stable domain concepts and the framework-free business/application services around them:

- repository diff request
- base branch resolution result
- file diff
- review file
- review section
- review line
- syntax token
- character highlight range
- context expansion state

Suggested section model:

- `ChangedSection`
- `CollapsedContextSection`
- `ExpandedContextSection`

The core decides which lines are visible and where expansion affordances appear.

Initial core services:

- `ResolveBaseBranch`
- `ResolveStartupDecision`
- `LoadBranchReview`
- `BuildReviewFiles`
- `ExpandReviewSection`
- `CollapseReviewSection`

The key rule: core services return plain data structures that a TUI, a future plugin, or tests can consume.

### `internal/ports/`

Ports are sibling to the core and define boundary contracts.

Initial outbound ports:

- `GitDiffLoader`
- `BaseBranchResolver`
- `StartupStateReader` for smart default launch
- `FileContentReader`
- `SyntaxTokenizer`
- `ReviewCallbackPublisher` (reserved for later integration)

For v1, the concrete git adapter should be built on top of `github.com/go-git/go-git/v5`.
Use it for repository open, HEAD/base ref resolution, remote-ref inspection, worktree status, and commit/tree access.

If a core service needs external data or side effects, it depends on a port.

## TUI composition

The Bubble Tea app should be an orchestrator, not a god object.

### Thin app shell

`internal/adapters/in/tui/` should mainly:

- hold the current screen-level state
- route key events to actions
- delegate rendering to components
- trigger app use cases

### Reusable components

Put reusable pieces under `internal/adapters/in/tui/component/`.

Initial components:

- `filelist` — changed files navigation and stats (standby; current TUI keeps one sequential viewport)
- `reviewpane` — virtualized review document pane in `internal/adapters/in/tui/review_pane.go`
- `searchpane` — floating file-find (`f`) and reference grep (`/`) over the review viewport
- `contextbar` — expand above / below / all controls
- `statusbar` — mode, file counts, branch info, hints
- `help` — key bindings and action descriptions

Each component should expose a small API, for example:

- `Update(action) result`
- `View(model) string`
- optional focused state/query helpers

The root model should compose components instead of embedding all rendering and key handling in one file.

### Presenter layer

`internal/adapters/in/tui/presenter/` converts core review data into a typed review document.

This projection is deliberately terminal-independent: no Bubble Tea, Bubbles, Lip Gloss, ANSI strings, or theme styles. It owns:

- section flattening into typed rows
- file and line anchors
- expander row indexes
- selectable row metadata
- annotation row placement for local comments, remote threads, and inline editors
- per-file line-number widths and file stats

The presenter may depend on `core` domain types because it projects review-domain data for the TUI adapter, but it must not depend on rendering packages. This keeps cursor mapping, search jumps, selection/copy, comment ranges, and context expansion metadata out of both the core and terminal renderer.

### Render layer

`internal/adapters/in/tui/render/` owns Lip Gloss-specific behavior:

- line number rendering
- diff backgrounds
- syntax token painting
- character highlight painting
- review row rendering from presenter rows
- context expander labels and selected-expander styling
- annotation chrome for local comments, remote threads, and inline editor rows
- cached rendering of expensive syntax-highlighted diff lines

The render layer accepts typed presenter rows plus visual state. It must not rebuild the review projection, mutate core review data, or decide navigation behavior.

### Virtualized review pane

`internal/adapters/in/tui/review_pane.go` is the viewport replacement for the review document. It owns:

- width, height, y-offset, scroll percent, and cursor visibility
- visible-row selection for `View()`
- gutter composition and content truncation
- renderer width synchronization after resize

The pane renders only visible rows. It receives a complete typed row list for anchors and cursor math, but it must not pre-render or store all styled review lines. Expander selection, cursor row, selected ranges, and comment-range markers are visual state passed to the renderer; cursor movement must not rebuild the projection just to update these highlights.

### Invalidation rules

The typed presenter document is rebuilt only when the review structure changes:

- diff files are loaded or reloaded
- context lines expand/collapse
- local comments, remote threads, or the active editor change annotation rows
- terminal width changes, measured in terminal character columns, when the change affects editor wrapping or row metadata

For the current implementation, every terminal-width change rebuilds the typed presenter document because inline editor wrapping is width-dependent row metadata. Diff lines do not soft-wrap in the review pane, and the line-number column width is derived from review line numbers rather than terminal width, so those do not create separate width breakpoints. If future rendering adds diff-line wrapping, the exact rebuild signals should be: a change in computed editor wrapping, a change in line-number column width, or another row metadata change. Simple cursor movement, selection changes, and nearest-expander highlight changes update visual state only. The syntax-highlighted line cache is cleared or replaced when diff content, syntax tokens, or width-sensitive line-number metadata changes.

## Highlighting design retrieved from `cue`

### 1. Syntax tokenization

Use a Chroma-backed adapter that:

- caches lexers by extension or filename
- tokenizes the whole file at once
- splits the token stream back into per-line token slices
- maps Chroma token types to a smaller semantic set like:
  - `keyword`
  - `function`
  - `type`
  - `name`
  - `string`
  - `number`
  - `comment`
  - `operator`
  - `punctuation`
  - `text`
- keeps that Chroma -> semantic-token mapping inside the syntax adapter
- starts with explicit mappings such as:
  - `Keyword* -> keyword`
  - `Name.Function* -> function`
  - `Name.Class*` and `Name.Builtin* -> type`
  - `Literal.String* -> string`
  - `Literal.Number* -> number`

This comes from the earlier `cue` prototype repository's Chroma syntax adapter.

### 2. Background-preserving syntax rendering

The core exposes plain tokens and line metadata.

The renderer:

- chooses the base diff background for the row
- applies syntax foreground colors token by token
- re-applies the diff background on each token render

This is the key idea from the earlier `cue` prototype repository's full-file renderer.

### 3. Character-level diff emphasis

For paired delete/add lines, compute inline highlight ranges and render the changed spans with stronger colors while preserving the line background.

The first iteration can use the simple prefix/suffix approach proven in the earlier `cue` prototype repository, with room to swap in the LCS variant later.

## v1 boundaries

Included:

- branch vs default-branch diff acquisition
- go-git-backed repository access
- GitHub-style per-file review sections
- collapsible unchanged context with explicit expansion actions
- unified review rendering in TUI
- floating file find and reference grep with jump-to-result behavior
- syntax coloring for displayed code
- inline character highlights for paired add/delete lines

Deferred:

- review/comment callbacks back into agents
- alternate diff targets and extra CLI subcommands
- multi-session integrations beyond the first Pi plugin adapter
- side-by-side diff mode
