# scratch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `scratch`, a single Go binary that edits one per-worktree markdown scratchpad (`$PWD/.scratch.md`) in a keyboard-first TUI with autosave and non-destructive external-change reload.

**Architecture:** A thin, essentially stateless editor over one file — the file is the source of truth, the TUI is a window onto it. A pure `internal/notes` package holds all file I/O and the reload-decision logic (fully unit-testable); `internal/tui` holds the Bubble Tea model and the fsnotify watcher; `main.go` dispatches CLI subcommands. No daemon, no DB, no required config.

**Tech Stack:** Go 1.26; Charm stack — Bubble Tea (runtime), bubbles/textarea (edit surface), lipgloss (Catppuccin Mocha chrome); fsnotify (file watching).

## Global Constraints

- Go version floor: **1.26** (repo has go1.26.4).
- Module path: **`github.com/schuettc/scratch`**. Binary name: **`scratch`**.
- Notes file: **`$PWD/.scratch.md`** (per-worktree). Missing file is treated as empty; created on first write.
- **All writes are atomic**: write a temp file in the same directory, then `rename` over the target. A concurrent reader must never see a half-written file.
- **Never silently clobber**: an external change while the buffer has unsaved edits must flag, not overwrite.
- Styling: **Catppuccin Mocha** palette (hex values given in Task 6). Dirty indicator `●`, clean `○`.
- No new dependencies beyond: `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/bubbles`, `github.com/charmbracelet/lipgloss`, `github.com/fsnotify/fsnotify`. (glamour/preview is explicitly out of scope.)
- Each task ends with a passing test run and a commit.

---

## File Structure

- **Create:** `go.mod`, `go.sum` (via tooling)
- **Create:** `internal/notes/notes.go` — `Path`, `Read`, `Write`, `Append`, `Classify`, `Action` type
- **Create:** `internal/notes/notes_test.go`
- **Create:** `internal/tui/model.go` — `Model`, `New`, `Init`, `Update`, `View`, messages, styles
- **Create:** `internal/tui/model_test.go`
- **Create:** `internal/tui/watcher.go` — `watchCmd`, `Run`
- **Create:** `main.go` — `run`, `main`
- **Create:** `README.md` — usage + manual smoke checklist

---

## Task 1: Project scaffold + notes.Path

**Files:**
- Create: `go.mod`
- Create: `internal/notes/notes.go`
- Test: `internal/notes/notes_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `notes.Path(cwd string) string` → returns `filepath.Join(cwd, ".scratch.md")`.

- [ ] **Step 1: Initialize the module**

```bash
cd /Users/courtschuett/GitHub/schuettc/scratch
go mod init github.com/schuettc/scratch
```

- [ ] **Step 2: Write the failing test**

Create `internal/notes/notes_test.go`:

```go
package notes

import (
	"path/filepath"
	"testing"
)

func TestPath(t *testing.T) {
	got := Path("/tmp/work")
	want := filepath.Join("/tmp/work", ".scratch.md")
	if got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/notes/`
Expected: FAIL — build error, `undefined: Path`.

- [ ] **Step 4: Write minimal implementation**

Create `internal/notes/notes.go`:

```go
// Package notes is the file layer for the scratch scratchpad: path
// resolution, atomic reads/writes, append, and the reload decision.
package notes

import "path/filepath"

// Path returns the scratchpad file path for a given working directory.
func Path(cwd string) string {
	return filepath.Join(cwd, ".scratch.md")
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/notes/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go.mod internal/notes/notes.go internal/notes/notes_test.go
git commit -m "feat: scaffold module and notes.Path"
```

---

## Task 2: Atomic Write + Read round-trip

**Files:**
- Modify: `internal/notes/notes.go`
- Test: `internal/notes/notes_test.go`

**Interfaces:**
- Consumes: `Path` (from Task 1).
- Produces:
  - `notes.Read(path string) (string, error)` → contents, or `("", nil)` if the file does not exist.
  - `notes.Write(path, content string) error` → atomic (temp + rename); leaves the temp file and returns a descriptive error only if `rename` fails.

- [ ] **Step 1: Write the failing tests**

Append to `internal/notes/notes_test.go`:

```go
func TestReadMissingIsEmpty(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".scratch.md")
	got, err := Read(p)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got != "" {
		t.Fatalf("Read() = %q, want empty", got)
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".scratch.md")
	const content = "# notes\n\n- one\n- two\n"
	if err := Write(p, content); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	got, err := Read(p)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got != content {
		t.Fatalf("round-trip = %q, want %q", got, content)
	}
	// No temp files left behind after a successful write.
	tmps, _ := filepath.Glob(filepath.Join(dir, ".scratch-*.tmp"))
	if len(tmps) != 0 {
		t.Fatalf("leftover temp files: %v", tmps)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/notes/`
Expected: FAIL — `undefined: Read`, `undefined: Write`.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/notes/notes.go` (and extend the import block):

```go
import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)
```

```go
// Read returns the file contents. A missing file is not an error: it
// returns ("", nil).
func Read(path string) (string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Write atomically replaces path's contents: it writes a temp file in the
// same directory, fsyncs, then renames over path. On write failure the temp
// file is cleaned up; on rename failure the temp file is kept and its path is
// reported so no content is lost.
func Write(path, content string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".scratch-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	write := func() error {
		if _, err := tmp.WriteString(content); err != nil {
			return err
		}
		if err := tmp.Sync(); err != nil {
			return err
		}
		return tmp.Close()
	}
	if err := write(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename %s to %s (temp kept): %w", tmpName, path, err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/notes/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/notes/notes.go internal/notes/notes_test.go
git commit -m "feat: atomic Write and Read for notes"
```

---

## Task 3: Append

**Files:**
- Modify: `internal/notes/notes.go`
- Test: `internal/notes/notes_test.go`

**Interfaces:**
- Consumes: `Read`, `Write` (from Task 2).
- Produces: `notes.Append(path, line string) error` → appends `line` plus a trailing newline atomically, ensuring a newline separates it from prior content; creates the file if missing.

- [ ] **Step 1: Write the failing tests**

Append to `internal/notes/notes_test.go`:

```go
func TestAppendToMissingCreates(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".scratch.md")
	if err := Append(p, "first"); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	got, _ := Read(p)
	if got != "first\n" {
		t.Fatalf("Append() = %q, want %q", got, "first\n")
	}
}

func TestAppendAddsSeparatingNewline(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".scratch.md")
	if err := Write(p, "existing"); err != nil { // no trailing newline
		t.Fatalf("Write() error = %v", err)
	}
	if err := Append(p, "next"); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	got, _ := Read(p)
	if got != "existing\nnext\n" {
		t.Fatalf("Append() = %q, want %q", got, "existing\nnext\n")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/notes/`
Expected: FAIL — `undefined: Append`.

- [ ] **Step 3: Write minimal implementation**

Add `"strings"` to the import block, then add to `internal/notes/notes.go`:

```go
// Append atomically adds line (with a trailing newline) to the file,
// inserting a separating newline if the existing content lacks one.
func Append(path, line string) error {
	cur, err := Read(path)
	if err != nil {
		return err
	}
	if cur != "" && !strings.HasSuffix(cur, "\n") {
		cur += "\n"
	}
	return Write(path, cur+line+"\n")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/notes/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/notes/notes.go internal/notes/notes_test.go
git commit -m "feat: atomic Append for notes"
```

---

## Task 4: Classify — the reload decision

**Files:**
- Modify: `internal/notes/notes.go`
- Test: `internal/notes/notes_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `notes.Action` — an int enum with exported constants `notes.Ignore`, `notes.Reload`, `notes.Flag`.
  - `notes.Classify(diskContent, lastWritten string, dirty bool) Action` → the whole conflict-safety decision:
    - `diskContent == lastWritten` → `Ignore` (it's our own write echoing back).
    - else `dirty` → `Flag` (unsaved edits — never clobber).
    - else → `Reload` (buffer clean, real external change).

- [ ] **Step 1: Write the failing tests**

Append to `internal/notes/notes_test.go`:

```go
func TestClassify(t *testing.T) {
	cases := []struct {
		name        string
		disk, last  string
		dirty       bool
		want        Action
	}{
		{"self-write echo ignored", "abc", "abc", false, Ignore},
		{"self-write echo ignored even if dirty", "abc", "abc", true, Ignore},
		{"external change while clean reloads", "new", "old", false, Reload},
		{"external change while dirty flags", "new", "old", true, Flag},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Classify(c.disk, c.last, c.dirty); got != c.want {
				t.Fatalf("Classify(%q,%q,%v) = %v, want %v",
					c.disk, c.last, c.dirty, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/notes/`
Expected: FAIL — `undefined: Classify`, `undefined: Action`.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/notes/notes.go`:

```go
// Action is the decision for an observed on-disk change.
type Action int

const (
	// Ignore means the change is our own write echoing back — do nothing.
	Ignore Action = iota
	// Reload means load the disk version into a clean buffer.
	Reload
	// Flag means there are unsaved local edits — surface a "changed on
	// disk" indicator but do not overwrite.
	Flag
)

// Classify decides what to do when the file changes on disk. lastWritten is
// the content the editor believes is on disk (last saved or loaded).
func Classify(diskContent, lastWritten string, dirty bool) Action {
	if diskContent == lastWritten {
		return Ignore
	}
	if dirty {
		return Flag
	}
	return Reload
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/notes/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/notes/notes.go internal/notes/notes_test.go
git commit -m "feat: Classify reload decision"
```

---

## Task 5: CLI dispatch (path / print / append)

**Files:**
- Create: `main.go`
- Test: `main_test.go`

**Interfaces:**
- Consumes: `notes.Path`, `notes.Read`, `notes.Append`.
- Produces:
  - `run(cwd string, args []string, stdout io.Writer) int` — dispatches subcommands, returns an exit code. `args` excludes the program name. No args → calls `tui.Run(path)` (wired in Task 7; a temporary stub returns 0 here and is replaced in Task 7).
  - `main()` — resolves cwd and calls `run`, then `os.Exit`.

- [ ] **Step 1: Write the failing tests**

Create `main_test.go`:

```go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPath(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if code := run(dir, []string{"path"}, &out); code != 0 {
		t.Fatalf("run path exit = %d, want 0", code)
	}
	want := filepath.Join(dir, ".scratch.md")
	if strings.TrimSpace(out.String()) != want {
		t.Fatalf("run path = %q, want %q", out.String(), want)
	}
}

func TestRunPrint(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".scratch.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := run(dir, []string{"print"}, &out); code != 0 {
		t.Fatalf("run print exit = %d, want 0", code)
	}
	if out.String() != "hello\n" {
		t.Fatalf("run print = %q, want %q", out.String(), "hello\n")
	}
}

func TestRunAppend(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if code := run(dir, []string{"append", "a line"}, &out); code != 0 {
		t.Fatalf("run append exit = %d, want 0", code)
	}
	got, _ := os.ReadFile(filepath.Join(dir, ".scratch.md"))
	if string(got) != "a line\n" {
		t.Fatalf("after append = %q, want %q", got, "a line\n")
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var out bytes.Buffer
	if code := run(t.TempDir(), []string{"bogus"}, &out); code != 2 {
		t.Fatalf("run bogus exit = %d, want 2", code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test .`
Expected: FAIL — `undefined: run` (no `main.go` yet).

- [ ] **Step 3: Write minimal implementation**

Create `main.go`. Note the temporary TUI stub — Task 7 replaces the `case len(args)==0` branch with the real `tui.Run`.

```go
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/schuettc/scratch/internal/notes"
)

func run(cwd string, args []string, stdout io.Writer) int {
	path := notes.Path(cwd)

	if len(args) == 0 {
		// Replaced in Task 7 with: return tui.Run(path)
		fmt.Fprintln(stdout, "TUI not wired yet")
		return 0
	}

	switch args[0] {
	case "path":
		fmt.Fprintln(stdout, path)
		return 0
	case "print":
		content, err := notes.Read(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Fprint(stdout, content)
		return 0
	case "append":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: scratch append <text>")
			return 2
		}
		if err := notes.Append(path, args[1]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		return 2
	}
}

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(run(cwd, os.Args[1:], os.Stdout))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS (notes + main packages).

- [ ] **Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: CLI dispatch for path/print/append"
```

---

## Task 6: TUI model — editor, autosave, chrome, keybinds

**Files:**
- Create: `internal/tui/model.go`
- Test: `internal/tui/model_test.go`

**Interfaces:**
- Consumes: `notes.Read`, `notes.Write`, `notes.Classify` and the `notes.Action` constants.
- Produces (used by Task 7):
  - `tui.Model` struct with an exported field `WatchCmd tea.Cmd` (set by `Run` to re-subscribe the watcher; nil-safe when unset).
  - `tui.New(path string) Model` — reads the file, builds a focused textarea.
  - `tui.DiskChangeMsg{Content string}` — the message the watcher emits (defined here so the model can handle it; produced in Task 7).
  - `Model.Init() tea.Cmd`, `Model.Update(tea.Msg) (tea.Model, tea.Cmd)`, `Model.View() string`.

- [ ] **Step 1: Add Charm dependencies**

```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/bubbles@latest
go get github.com/charmbracelet/lipgloss@latest
```

- [ ] **Step 2: Write the failing tests**

Create `internal/tui/model_test.go`:

```go
package tui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/schuettc/scratch/internal/notes"
)

// runCmd executes a tea.Cmd (possibly a batch) and returns the messages it
// produces, so tests can drive the model deterministically.
func drainMsg(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestTypingMarksDirtyAndSchedulesSave(t *testing.T) {
	m := New(filepath.Join(t.TempDir(), ".scratch.md"))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = next.(Model)
	if !m.dirty {
		t.Fatal("typing should mark buffer dirty")
	}
	if cmd == nil {
		t.Fatal("typing should schedule a save (non-nil cmd)")
	}
}

func TestCtrlSWritesFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".scratch.md")
	m := New(p)
	// type something first
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h', 'i'}})
	m = next.(Model)
	// ctrl+s returns a save command; execute it
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	msg := drainMsg(cmd)
	if _, ok := msg.(savedMsg); !ok {
		t.Fatalf("ctrl+s should yield savedMsg, got %T", msg)
	}
	got, _ := notes.Read(p)
	if got != "hi" {
		t.Fatalf("file after ctrl+s = %q, want %q", got, "hi")
	}
}

func TestDiskChangeWhileCleanReloads(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".scratch.md")
	m := New(p) // clean, empty buffer, lastWritten ""
	next, _ := m.Update(DiskChangeMsg{Content: "from claude"})
	m = next.(Model)
	if m.textarea.Value() != "from claude" {
		t.Fatalf("clean disk change should reload, got %q", m.textarea.Value())
	}
	if m.diskChanged {
		t.Fatal("reload should not set the changed-on-disk flag")
	}
}

func TestDiskChangeWhileDirtyFlags(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".scratch.md")
	m := New(p)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m', 'e'}})
	m = next.(Model) // now dirty with "me"
	next, _ = m.Update(DiskChangeMsg{Content: "from claude"})
	m = next.(Model)
	if !m.diskChanged {
		t.Fatal("dirty disk change should set the changed-on-disk flag")
	}
	if m.textarea.Value() != "me" {
		t.Fatalf("dirty disk change must not clobber, got %q", m.textarea.Value())
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL — `undefined: New`, `undefined: Model`, etc.

- [ ] **Step 4: Write minimal implementation**

Create `internal/tui/model.go`:

```go
package tui

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/schuettc/scratch/internal/notes"
)

const saveDebounce = 500 * time.Millisecond

// Catppuccin Mocha palette.
var (
	colMauve    = lipgloss.Color("#cba6f7")
	colGreen    = lipgloss.Color("#a6e3a1")
	colRed      = lipgloss.Color("#f38ba8")
	colSubtext0 = lipgloss.Color("#a6adc8")

	headerStyle = lipgloss.NewStyle().Foreground(colMauve).Bold(true)
	cleanStyle  = lipgloss.NewStyle().Foreground(colSubtext0)
	dirtyStyle  = lipgloss.NewStyle().Foreground(colGreen)
	flagStyle   = lipgloss.NewStyle().Foreground(colRed)
	footerStyle = lipgloss.NewStyle().Foreground(colSubtext0)
	errStyle    = lipgloss.NewStyle().Foreground(colRed)
)

// DiskChangeMsg is emitted by the watcher when the file changes on disk.
type DiskChangeMsg struct{ Content string }

type saveMsg struct{ gen int }
type savedMsg struct {
	content string
	err     error
}

// Model is the Bubble Tea model for the scratch editor.
type Model struct {
	// WatchCmd re-subscribes the fsnotify watcher; set by Run. Nil-safe.
	WatchCmd tea.Cmd

	path        string
	textarea    textarea.Model
	lastWritten string // what we believe is on disk
	dirty       bool
	diskChanged bool
	saveErr     string
	gen         int // debounce generation
}

// New builds a model with the file's current contents loaded.
func New(path string) Model {
	content, _ := notes.Read(path)

	ta := textarea.New()
	ta.Placeholder = "notes…"
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.CharLimit = 0
	ta.SetValue(content)
	ta.Focus()

	return Model{
		path:        path,
		textarea:    ta,
		lastWritten: content,
	}
}

func (m Model) Init() tea.Cmd {
	return m.WatchCmd // may be nil (e.g. watcher unavailable)
}

func (m Model) scheduleSave() tea.Cmd {
	gen := m.gen
	return tea.Tick(saveDebounce, func(time.Time) tea.Msg {
		return saveMsg{gen: gen}
	})
}

func (m Model) saveNow() tea.Cmd {
	content := m.textarea.Value()
	path := m.path
	return func() tea.Msg {
		return savedMsg{content: content, err: notes.Write(path, content)}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.textarea.SetWidth(msg.Width)
		h := msg.Height - 2 // header + footer
		if h < 1 {
			h = 1
		}
		m.textarea.SetHeight(h)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+q", "esc":
			return m, tea.Sequence(m.saveNow(), tea.Quit)
		case "ctrl+s":
			return m, m.saveNow()
		case "ctrl+r":
			if m.diskChanged {
				content, _ := notes.Read(m.path)
				m.textarea.SetValue(content)
				m.lastWritten = content
				m.dirty = false
				m.diskChanged = false
			}
			return m, nil
		}
		before := m.textarea.Value()
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		cmds := []tea.Cmd{cmd}
		if m.textarea.Value() != before {
			m.dirty = true
			m.gen++
			cmds = append(cmds, m.scheduleSave())
		}
		return m, tea.Batch(cmds...)

	case saveMsg:
		if msg.gen == m.gen && m.dirty {
			return m, m.saveNow()
		}
		return m, nil

	case savedMsg:
		if msg.err != nil {
			m.saveErr = msg.err.Error()
			return m, nil
		}
		m.saveErr = ""
		m.lastWritten = msg.content
		m.dirty = m.textarea.Value() != msg.content
		return m, nil

	case DiskChangeMsg:
		switch notes.Classify(msg.Content, m.lastWritten, m.dirty) {
		case notes.Reload:
			m.textarea.SetValue(msg.Content)
			m.lastWritten = msg.Content
			m.dirty = false
			m.diskChanged = false
		case notes.Flag:
			m.diskChanged = true
		case notes.Ignore:
		}
		return m, m.WatchCmd // re-subscribe (nil-safe)
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	name := filepath.Base(filepath.Dir(m.path))

	dot := cleanStyle.Render("○")
	if m.dirty {
		dot = dirtyStyle.Render("●")
	}
	header := headerStyle.Render(name) + " " + dot
	if m.diskChanged {
		header += "  " + flagStyle.Render("● changed on disk (ctrl+r to reload)")
	}

	footer := footerStyle.Render("ctrl+s save · ctrl+r reload · ctrl+q quit")
	if m.saveErr != "" {
		footer = errStyle.Render("save error: " + m.saveErr)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, m.textarea.View(), footer)
}

var _ = fmt.Sprintf // retained for future footer formatting
```

Note: the `var _ = fmt.Sprintf` line keeps the `fmt` import if you trim the footer; if `fmt` is otherwise used, delete that line and the import. Run `go vet` to confirm imports are clean before committing.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/tui/`
Expected: PASS (4 tests). If `fmt` is unused, remove the import and the `var _ = fmt.Sprintf` line, then re-run.

- [ ] **Step 6: Commit**

```bash
go mod tidy
git add go.mod go.sum internal/tui/model.go internal/tui/model_test.go
git commit -m "feat: TUI model with autosave, reload, and chrome"
```

---

## Task 7: fsnotify watcher + wire TUI entry + docs

**Files:**
- Create: `internal/tui/watcher.go`
- Modify: `main.go` (replace the no-args TUI stub)
- Create: `README.md`

**Interfaces:**
- Consumes: `tui.New`, `tui.Model.WatchCmd`, `tui.DiskChangeMsg`, `notes.Read`, `notes.Path`.
- Produces: `tui.Run(path string) int` — builds the model, watches the file's directory (survives atomic renames), runs the Bubble Tea program; returns an exit code.

- [ ] **Step 1: Add fsnotify**

```bash
go get github.com/fsnotify/fsnotify@latest
```

- [ ] **Step 2: Write the watcher and entry point**

Create `internal/tui/watcher.go`:

```go
package tui

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
	"github.com/schuettc/scratch/internal/notes"
)

// watchCmd blocks on the watcher until a relevant event for path arrives,
// then returns a DiskChangeMsg with the file's current contents. The model
// re-issues this command to keep listening. We watch the directory (not the
// file) so the watch survives atomic renames, and filter to our file.
func watchCmd(w *fsnotify.Watcher, path string) tea.Cmd {
	return func() tea.Msg {
		for {
			select {
			case event, ok := <-w.Events:
				if !ok {
					return nil
				}
				if filepath.Base(event.Name) != filepath.Base(path) {
					continue
				}
				if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
					continue
				}
				content, _ := notes.Read(path)
				return DiskChangeMsg{Content: content}
			case _, ok := <-w.Errors:
				if !ok {
					return nil
				}
			}
		}
	}
}

// Run launches the editor on path. If the watcher cannot be created, the
// editor still runs — just without live auto-reload.
func Run(path string) int {
	m := New(path)

	if w, err := fsnotify.NewWatcher(); err == nil {
		defer w.Close()
		if err := w.Add(filepath.Dir(path)); err == nil {
			m.WatchCmd = watchCmd(w, path)
		}
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
```

- [ ] **Step 3: Wire the entry point into main.go**

In `main.go`, add the import and replace the no-args stub.

Add to the import block:

```go
	"github.com/schuettc/scratch/internal/tui"
```

Replace:

```go
	if len(args) == 0 {
		// Replaced in Task 7 with: return tui.Run(path)
		fmt.Fprintln(stdout, "TUI not wired yet")
		return 0
	}
```

with:

```go
	if len(args) == 0 {
		return tui.Run(path)
	}
```

- [ ] **Step 4: Build and verify it compiles**

Run: `go build ./... && go vet ./...`
Expected: no output (success).

- [ ] **Step 5: Run the full test suite**

Run: `go test ./...`
Expected: PASS across `notes`, `tui`, and `main`.

- [ ] **Step 6: Write the README with the manual smoke checklist**

Create `README.md`:

````markdown
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

Keys: type to edit · `ctrl+s` save · `ctrl+r` reload from disk · `ctrl+q`/`esc` quit.
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
````

- [ ] **Step 7: Commit**

```bash
go mod tidy
git add go.mod go.sum internal/tui/watcher.go main.go README.md
git commit -m "feat: fsnotify watcher, wire TUI entry, and README"
```

---

## Task 8: End-to-end verification

**Files:** none (verification only).

- [ ] **Step 1: Install locally**

```bash
go install github.com/schuettc/scratch
```

- [ ] **Step 2: Verify non-TUI subcommands end-to-end**

```bash
cd $(mktemp -d)
scratch path      # prints <dir>/.scratch.md
scratch append "first note"
scratch print     # prints "first note"
```

Expected: `path` prints the temp dir's `.scratch.md`; `print` shows `first note`.

- [ ] **Step 3: Run the manual TUI smoke checklist**

Work through all four items in the README "Manual smoke checklist" section. All must pass.

- [ ] **Step 4: Final full test + vet**

```bash
go test ./... && go vet ./...
```

Expected: PASS, no vet warnings.

---

## Self-Review

**Spec coverage:**
- Editor / full free-form editing → Task 6 (textarea). ✓
- Autosave (debounce, on-quit, atomic) → Task 6 (`scheduleSave`/`saveNow` + `ctrl+q` flush) + Task 2 (atomic Write). ✓
- Watcher clean→reload / dirty→flag / self-write suppression → Task 4 (`Classify`) + Task 6 (handling) + Task 7 (fsnotify producer). ✓
- Chrome (header name + dot + flag, footer hint, error line) → Task 6 (`View`). ✓
- CLI `print`/`append`/`path` + TUI default → Task 5 + Task 7. ✓
- File location/scoping (`$PWD/.scratch.md`, missing→empty, create on write) → Task 1 (`Path`), Task 2 (`Read` missing→empty, `Write` creates). ✓
- Error handling (missing file, write failure surfaced, atomic-rename keeps temp, fsnotify degrade) → Task 2 (Write error text), Task 6 (`saveErr` footer), Task 7 (watcher-unavailable degrade). ✓
- Testing (atomic round-trip, append, print/path, reload logic, self-write suppression) → Tasks 2–6 tests + Task 8 smoke. ✓
- **Out of scope by spec:** dotfiles/tmux integration (separate follow-on), glamour preview (later). Not planned here — correct.

**Placeholder scan:** No TBD/TODO/"handle edge cases"; every code step shows full code; the one intentional stub (Task 5 no-args branch) is explicitly flagged and replaced in Task 7. ✓

**Type consistency:** `notes.Action`/`Ignore`/`Reload`/`Flag`, `notes.Classify`, `notes.Read/Write/Append/Path`, `tui.Model.WatchCmd`, `tui.DiskChangeMsg{Content}`, `tui.New`, `tui.Run`, `run(cwd,args,stdout)` used identically across tasks. ✓
