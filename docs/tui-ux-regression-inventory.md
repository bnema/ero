# TUI UX regression inventory

This inventory captures review UI/UX behavior that must remain stable during the virtualized rendering refactor for issue #4.

## Review document rendering

- Files render as one sequential review document sorted by path.
- Each file keeps its header, stats, separator rule, changed lines, visible context lines, and context expander rows.
- Diff lines keep old/new line-number columns, +/- markers, diff backgrounds, and syntax foreground colors.
- Empty reviews show the Review title and `No files to review` message.
- Status and review content fit the configured terminal width.

## Cursor, gutter, and highlights

- `j`/`down` and `k`/`up` move between selectable review rows.
- `home`, `end`, `pgup`, and `pgdown` keep the same absolute cursor and viewport semantics.
- The viewport follows the cursor without scrolling until the cursor leaves the visible window.
- The gutter shows the cursor arrow on the active row.
- Selection mode shows the selected range gutter marker and row highlight.
- Comment ranges and the active inline editor range keep their dedicated gutter markers and row highlight.
- The status bar active location follows the cursor and shows file/line when available.

## Context expanders

- Context expander labels keep their current wording for file start, file end, between changes, and only-section contexts.
- Only the nearest expander in the current file is selected as the cursor moves.
- The selected expander highlight is visible and follows nearest-context tracking.
- `enter` expands the context under the cursor when the cursor is on an expander.
- `enter` expands above or below according to current edge/middle behavior:
  - file-start context expands below;
  - file-end context expands above;
  - between-changes context expands above unless the cursor is below the expander.
- `a` expands all context for the selected/under-cursor expander.
- Moving into a file without expanders clears stale context selection.

## Search and navigation

- `f` opens file search, filters files, and accepting a result jumps to the file header.
- `/` opens grep search, filters review lines, and accepting a result jumps to the matched line.
- Jumping to a hidden context grep match expands that context before moving the cursor.
- Jumping to an already visible context line preserves existing partial context expansion.
- `d` opens diff-mode search and accepting a mode triggers reload behavior.
- Search overlays keep their centered/floating behavior and keyboard navigation.

## Selection, copy, and review draft

- `s`/space toggles selection and `esc` clears it outside modal overlays.
- Plain copy uses the selected lines or current line and preserves diff markers.
- Metadata copy groups selected lines by file and includes file/line metadata plus fenced diff text.
- Copy feedback appears in the status bar and expires by feedback id.
- `C` copies the current review JSON.
- `x` clears the local review draft and cancels active editors.

## Inline comments and remote threads

- `c` opens an inline comment editor on the current line or selected contiguous range.
- The inline editor requests keyboard enhancements while active.
- `esc` cancels the editor.
- `ctrl+enter`/submit adds the comment.
- Without publish providers, submitting copies review JSON automatically.
- With publish providers, submitting keeps the draft locally and prompts publish.
- Comments render below their anchored range, indented under the code content column.
- Unmapped remote threads render as review messages; mapped threads render below their anchored line.
- Published provider refs remain attached to draft comments.

## Publish, overlays, and status bar

- `?` opens help and `esc`/`?` closes it; `q`/`ctrl+c` quits from help.
- Help, search, and publish overlays compose over the review content without changing underlying cursor state.
- `P` opens publish review only when providers support publishing.
- Publish overlay supports up/down focus, space toggle, enter publish, and escape cancel behavior.
- Status bar keeps app name, diff mode, file count, provider count/unavailable feedback, active location, hints, copy/publish messages, and scroll percent.

## Refactor guardrails

- New rendering code may change internal data structures, but it must not remove any behavior above.
- During refactor phases, the characterization tests should remain green before deleting the old full-render path.
- Performance work must be additive to UX fidelity: a faster path that drops highlights, cursor tracking, context selection, or overlays is a regression.
