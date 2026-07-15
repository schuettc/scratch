# Changelog

All notable changes to `scratch` are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/), and versions follow
[Semantic Versioning](https://semver.org/).

## [Unreleased]

- Added a Claude Code explainer skill (`.claude/skills/scratch/`), this changelog,
  and a GitHub Pages site.

## [0.1.2] — 2026-07-14

### Added
- Clear the scratchpad with `ctrl+x` — arms a `clear all? y/n` confirmation in the
  status line; `y` wipes the buffer and autosaves empty, anything else cancels
  (guards against a one-key wipe of your notes).

## [0.1.1] — 2026-07-14

### Changed
- Chrome redesign: a small filled title bar naming the pane
  (`scratch · <workspace> ●`) and a data status line showing the last **saved-at**
  time (`saved HH:MM`).

### Removed
- The empty-state `notes…` placeholder (it rendered highlighted/odd).
- The on-screen `ctrl+s · ctrl+r · ctrl+q` command hints — data over command hints.

## [0.1.0] — 2026-07-14

### Added
- First release: a per-worktree markdown scratchpad TUI that edits `$PWD/.scratch.md`.
- Debounced atomic autosave (temp-file + `rename`); saves are serialized so an
  overlapping stale write can't clobber newer content; the buffer is flushed on quit.
- Non-destructive external-change reload via fsnotify — `Classify` reloads when the
  buffer is clean, flags "changed on disk" when dirty (never clobbers), and ignores
  our own writes; the watcher watches the directory so it survives atomic renames.
- Keys: type to edit · `ctrl+s` save · `ctrl+r` reload · `ctrl+q`/`esc` quit.
- CLI subcommands: `scratch` (TUI), `scratch print`, `scratch append <text>`,
  `scratch path`.

[Unreleased]: https://github.com/schuettc/scratch/compare/v0.1.2...HEAD
[0.1.2]: https://github.com/schuettc/scratch/releases/tag/v0.1.2
[0.1.1]: https://github.com/schuettc/scratch/releases/tag/v0.1.1
[0.1.0]: https://github.com/schuettc/scratch/releases/tag/v0.1.0
