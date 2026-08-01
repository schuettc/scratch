package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// writePad is a pad that already exists on disk with content.
func writePad(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// The scenario this whole mechanism exists for: the editor starts before the
// coding agent does, so it opens the directory-keyed pad; the agent's
// SessionStart hook then stamps its identity and the editor must move to the
// session's pad on its own.
func TestAdoptsSessionPadWhenItAppears(t *testing.T) {
	dir := t.TempDir()
	dirPad := writePad(t, dir, "-work-repo.md", "directory pad\n")
	sessPad := writePad(t, dir, "sess-abc.md", "session pad\n")

	current := dirPad
	m := New(dirPad)
	m.Resolve = func() string { return current }

	if got := m.textarea.Value(); got != "directory pad\n" {
		t.Fatalf("opened with %q, want the directory pad", got)
	}

	// Tick before the stamp lands: nothing moves.
	next, cmd := m.Update(followTickMsg{})
	m = next.(Model)
	if m.path != dirPad {
		t.Fatalf("moved to %q with no stamp present", m.path)
	}
	if cmd == nil {
		t.Fatal("follow tick must reschedule itself")
	}

	// The hook stamps @harness_session; resolution now points at the session.
	current = sessPad
	next, _ = m.Update(followTickMsg{})
	m = next.(Model)

	if m.path != sessPad {
		t.Fatalf("path = %q, want the session pad", m.path)
	}
	if got := m.textarea.Value(); got != "session pad\n" {
		t.Fatalf("buffer = %q, want the session pad's content", got)
	}
	if m.dirty {
		t.Fatal("a freshly adopted pad must not start dirty")
	}
	if m.pathRef.get() != sessPad {
		t.Fatalf("watcher box = %q, want %q — the watcher would keep filtering on the old pad",
			m.pathRef.get(), sessPad)
	}
}

// Unsaved notes belong to the conversation they were typed in. Switching
// must flush them to the OLD pad, not carry them into the new one.
func TestSwitchFlushesUnsavedToOldPad(t *testing.T) {
	dir := t.TempDir()
	oldPad := writePad(t, dir, "old.md", "")
	newPad := writePad(t, dir, "new.md", "new pad\n")

	m := New(oldPad)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("typed here")})
	m = next.(Model)
	if !m.dirty {
		t.Fatal("precondition: buffer should be dirty")
	}

	m, _ = m.switchTo(newPad)

	saved, err := os.ReadFile(oldPad)
	if err != nil {
		t.Fatal(err)
	}
	if string(saved) != "typed here" {
		t.Fatalf("old pad = %q, want the unsaved edits flushed to it", saved)
	}
	if got := m.textarea.Value(); got != "new pad\n" {
		t.Fatalf("buffer = %q, want the new pad's content", got)
	}
	if m.dirty {
		t.Fatal("buffer should be clean after adopting the new pad")
	}
}

func TestSwitchToSamePathIsNoop(t *testing.T) {
	dir := t.TempDir()
	pad := writePad(t, dir, "same.md", "content\n")
	m := New(pad)
	m.textarea.SetValue("edited")
	m.dirty = true

	m, cmd := m.switchTo(pad)
	if cmd != nil {
		t.Fatal("no-op switch should not emit a command")
	}
	if got := m.textarea.Value(); got != "edited" {
		t.Fatalf("buffer = %q, want the edit preserved", got)
	}
	if !m.dirty {
		t.Fatal("no-op switch must not clear dirty")
	}
}

func TestSwitchToEmptyPathIsNoop(t *testing.T) {
	pad := writePad(t, t.TempDir(), "p.md", "x\n")
	m := New(pad)
	m, _ = m.switchTo("")
	if m.path != pad {
		t.Fatalf("path = %q, want unchanged — resolution failure must not move the editor", m.path)
	}
}

// A pad that cannot be read is not a reason to blank the operator's screen.
func TestSwitchToUnreadableStaysPut(t *testing.T) {
	dir := t.TempDir()
	pad := writePad(t, dir, "good.md", "good\n")
	bad := filepath.Join(dir, "unreadable")
	if err := os.Mkdir(bad, 0o755); err != nil { // a directory is not readable as a file
		t.Fatal(err)
	}

	m := New(pad)
	m, _ = m.switchTo(bad)
	if m.path != pad {
		t.Fatalf("path = %q, want unchanged", m.path)
	}
	if got := m.textarea.Value(); got != "good\n" {
		t.Fatalf("buffer = %q, want it left intact", got)
	}
}

func TestFollowDisabledWhenResolveNil(t *testing.T) {
	pad := writePad(t, t.TempDir(), "p.md", "x\n")
	m := New(pad)
	m.Resolve = nil

	next, cmd := m.Update(followTickMsg{})
	m = next.(Model)
	if m.path != pad {
		t.Fatalf("path = %q, want unchanged when following is disabled", m.path)
	}
	if cmd == nil {
		t.Fatal("tick must still reschedule so following can be enabled later")
	}
}

// ctrl+x arms a y/n that refers to the pad on screen. Swapping the pad out
// from under it would wipe the wrong one.
func TestNoSwitchWhileConfirmingClear(t *testing.T) {
	dir := t.TempDir()
	pad := writePad(t, dir, "a.md", "a\n")
	other := writePad(t, dir, "b.md", "b\n")

	m := New(pad)
	m.Resolve = func() string { return other }

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	m = next.(Model)
	if !m.confirmingClear {
		t.Fatal("precondition: ctrl+x should arm the confirmation")
	}

	next, cmd := m.Update(followTickMsg{})
	m = next.(Model)
	if m.path != pad {
		t.Fatalf("path = %q, want unchanged while a clear is armed", m.path)
	}
	if cmd == nil {
		t.Fatal("tick must reschedule so the switch happens once the confirm resolves")
	}
}

// Init must start the follow poll, or the editor never notices the stamp.
func TestInitSchedulesFollow(t *testing.T) {
	pad := writePad(t, t.TempDir(), "p.md", "")
	m := New(pad)
	if m.Init() == nil {
		t.Fatal("Init must schedule the follow tick")
	}
}
