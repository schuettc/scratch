# scratch

A fast, keyboard-first TUI for one per-worktree markdown scratchpad:
`$PWD/.scratch.md`. Autosaves, reloads external edits non-destructively, and
stays out of your way.

## Install

```bash
go install github.com/schuettc/scratch@latest
```

## Usage

```bash
scratch            # open the TUI editor on ./.scratch.md
scratch print      # print the file to stdout
scratch append "…" # atomically append a line (for hooks/scripts)
scratch path       # print the resolved file path
```

Keys: type to edit · `ctrl+s` save · `ctrl+r` reload from disk · `ctrl+x` clear (asks `y/n`) · `ctrl+q`/`esc` quit.
Autosave runs ~500ms after you stop typing, on quit, and on `ctrl+s`.

## Manual smoke checklist

1. In a dir with no `.scratch.md`: run `scratch`, type, wait ~1s, quit;
   `cat .scratch.md` shows your text.
2. With the TUI open and idle (clean), run `scratch append "x"` in another
   shell → the line appears live in the editor.
3. Type locally (don't save), then `scratch append "y"` elsewhere → header
   shows `● changed on disk`; your edits are intact; `ctrl+r` loads the disk
   version.
4. Quit with unsaved edits (`ctrl+q`) → they're flushed to disk.
