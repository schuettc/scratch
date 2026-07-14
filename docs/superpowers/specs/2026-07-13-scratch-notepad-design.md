# scratch — a per-workspace markdown scratchpad TUI

**Date:** 2026-07-13
**Status:** Design approved, ready for implementation plan

## Summary

`scratch` is a small, fast, keyboard-first terminal UI for editing a single
markdown notes file — one per workspace (git worktree / tmux session). It lives
in the top of the right-hand column of the existing Ghostty + tmux + yazi
workflow (notes on top, yazi below), giving a persistent, always-visible place
to jot "what I'm working on / what we're doing." The file is plain markdown on
disk, so Claude Code (and any other tool) can read and write it directly.

yazi is the *quality bar* for this project — instant, keyboard-driven, beautiful,
purpose-built — not a structural blueprint. `scratch` does exactly one thing:
edit one file, well.

## Goals

- A notes surface that is **effortless to edit** (delete, restructure, rewrite),
  so it actually stays current and accurate.
- **Zero ceremony**: open the pane and you're editing — no modes, no manual save.
- **Auto-refresh** when the file changes on disk (e.g. Claude writes to it),
  **without ever silently clobbering** in-progress edits.
- **Plain markdown file** as the single source of truth — greppable, diffable,
  portable, and editable by nvim as a fallback.
- Fits the existing tmux right-column layout and its agent-pane pinning.

## Non-goals (YAGNI)

- Not a multi-note notebook, not a file browser, not tags/search/metadata.
  One scratchpad per workspace, full stop. (That shape is what a DB would earn;
  we explicitly rejected it.)
- Not a WYSIWYG / live-rendered markdown editor. We edit raw markdown source.
  (Live rendering in a TUI is hard and buys little here. A read-only preview
  toggle is a possible *later* addition, out of scope for the MVP.)
- Not a daemon, not a config-file-driven app, no database.
- Not collaborative real-time co-editing (CRDTs). Conflicts are avoided by the
  autosave + reload-or-flag model below, not by merge machinery.

## Architecture

A single statically-linked Go binary built on the Charm stack:

- **Bubble Tea** — the TUI runtime / event loop.
- **bubbles/textarea** — the editable text surface.
- **lipgloss** — Catppuccin Mocha styling and layout.
- **fsnotify** — watch the notes file for external changes.
- **glamour** — *only if* we later add a preview toggle (not in MVP).

The binary is a thin, essentially stateless editor: **the file is the source of
truth**, the TUI is a window onto it. No daemon, no DB, no required config file.

### Notes file location & scoping

- The scratchpad is `$PWD/.scratch.md` — a file **in the current worktree**.
- Scoping is therefore automatic and per-worktree: each tmux session (one per
  worktree in the existing workflow) edits its own `.scratch.md`. "What we're
  doing" is feature-specific, which matches per-worktree scoping.
- `.scratch.md` is added to the user's **global gitignore** so it never pollutes
  any repo. A user who *wants* to commit notes in a specific repo can un-ignore
  it there.
- Bonus: because it lives in the worktree, the file is visible in the yazi pane
  directly below — discoverable and openable by other tools.
- If `$PWD/.scratch.md` does not exist, `scratch` creates it (empty) on first
  write.

## Components

1. **Editor model** — a `bubbles/textarea` holding the file contents; full
   free-form editing (insert, delete, restructure, rewrite). Soft-wrap on.
2. **Persistence** — autosave, no manual save:
   - Debounced save ~500ms after the last keystroke.
   - Save on focus-loss and on quit (flush before exit).
   - **Atomic write**: write to a temp file in the same dir, then `rename` over
     `.scratch.md`, so a concurrent reader (Claude, yazi preview) never sees a
     half-written file.
3. **Watcher** — `fsnotify` on `.scratch.md`:
   - External change while the **buffer is clean** → reload silently (you see
     Claude's edits appear).
   - External change while the **buffer is dirty** (unsaved local edits) → do
     **not** overwrite. Show a `● changed on disk` indicator and bind a reload
     key (e.g. `Ctrl-R`) to discard local edits and load the disk version.
     This is the whole conflict-safety story: never silently clobber.
   - Debounce/coalesce rapid fsnotify events; ignore events caused by our own
     atomic save (compare against last-written content / mtime).
4. **Chrome (lipgloss, Catppuccin Mocha)**:
   - Header line: workspace name (basename of `$PWD`) + a saved/dirty dot
     (`○` clean / `●` dirty) + a `changed on disk` flag when relevant.
   - Body: the textarea.
   - Footer: a one-line keybind hint.
5. **CLI subcommands** (single binary):
   - `scratch` — open the TUI on `$PWD/.scratch.md`.
   - `scratch print` — print the file to stdout (for piping / hooks).
   - `scratch append "text"` — atomic append a line (for hooks/skills/scripts).
   - `scratch path` — print the resolved notes-file path (useful for scripts).
   - Claude Code normally just edits the file directly with its own tools;
     `append` exists for non-interactive callers.

## Data flow

```
keystrokes → textarea buffer → debounced atomic save → .scratch.md
.scratch.md changed externally → fsnotify → (clean: reload) | (dirty: flag)
```

Both writers (you via the TUI, Claude via file edits) converge on the one file.
Autosave keeps the buffer clean most of the time, so external reloads are almost
always silent; the dirty-collision case is rare and handled non-destructively.

## Keybindings (MVP)

| Key | Action |
|-----|--------|
| (typing) | edit — inserts/deletes immediately |
| `Ctrl-S` | force save now (autosave also runs) |
| `Ctrl-R` | reload from disk (discard local edits) — surfaced when `changed on disk` |
| `Ctrl-Q` / `Esc` | flush save, then quit |

(Arrow/emacs motion keys come from `bubbles/textarea` defaults.)

## tmux / dotfiles integration (companion change in `~/dotfiles`)

The right 30% column becomes a vertical stack: **`scratch` (top) → `yazi`
(below)** → any subagent panes. Changes, all in the dotfiles repo:

1. **Pane launch** — in `config/zsh/06-tmux-autojoin.zsh` and the `proj`/`pt`
   launch path (`config/zsh/04-aliases.zsh` / wherever `__proj_launch` lives):
   after creating the yazi pane in the right column, split a `scratch` pane
   **above** it, sized so `scratch` gets a comfortable top slice of the column
   (e.g. yazi is split off the bottom of scratch, or scratch is split above
   yazi — same result). Keep yazi focused during its terminal probe as today.
2. **`prefix f` toggle** — in `.tmux.conf`, update the toggle to build/tear-down
   the **whole right column** (scratch + yazi), not just yazi.
3. **Agent-pin hooks** — the existing hooks pin the left pane to 70% for 3+ pane
   windows; that continues to work. Only the right-column internal split ratio
   needs retuning so scratch + yazi + agent panes share the column sensibly.
4. **Global gitignore** — add `.scratch.md`.
5. **Install** — add one line to fetch/build the binary (Brewfile `go install`
   entry, a brew tap, or an `install.sh` step), so a fresh machine gets
   `scratch` on `PATH`.

The dotfiles integration is a small follow-on change tracked separately from the
tool itself; this spec documents it so the two stay coordinated.

## Error handling

- Missing `.scratch.md` → treat as empty; create on first write.
- Read-only dir / write failure → surface a non-fatal error line in the footer;
  keep the buffer intact so nothing is lost; retry on next save.
- fsnotify unavailable / watch add fails → degrade to a periodic mtime poll (or
  no auto-reload) rather than crashing; the editor still works.
- Atomic write: if the `rename` fails, keep the temp file and report the path so
  no content is lost.

## Testing

- **Go unit tests:**
  - Atomic save round-trip (write → read back identical; temp file cleaned up).
  - `append` correctness (line appended atomically; existing content intact).
  - `print` / `path` output.
  - Reload logic: given (buffer-clean, external-change) → reload; given
    (buffer-dirty, external-change) → flag, do not overwrite.
  - fsnotify event coalescing / self-write suppression (fed synthetic events).
- **Manual TUI smoke checklist** (in the repo README / a test doc):
  1. Open in a worktree with no `.scratch.md` → empty editor; type → file
     created on disk after idle.
  2. Type, wait for autosave, `cat .scratch.md` → content matches.
  3. `scratch append "x"` from another shell while TUI open & clean → appears.
  4. Edit locally (dirty), then external-append → `changed on disk` flag shows,
     local edits preserved; `Ctrl-R` loads disk version.
  5. Quit with unsaved edits → they're flushed to disk.

## Rollout / sequencing

1. Build and test the `scratch` binary in its own repo (this repo).
2. `go install` it locally; verify it opens `.scratch.md` and behaves per the
   smoke checklist.
3. Land the dotfiles companion change (pane wiring, `prefix f`, gitignore,
   install line); verify the right column shows scratch-over-yazi and the toggle
   works.
