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

func TestOverlappingSavesSerializeAndDrainLatest(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".scratch.md")
	m := New(p)

	// Type "a" and start a save via ctrl+s.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = next.(Model)
	next, saveCmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(Model)
	if !m.saving {
		t.Fatal("ctrl+s should mark a save in flight")
	}

	// While that save is in flight, type "b" (buffer becomes "ab").
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = next.(Model)

	// A debounce tick for the new content must NOT start a second concurrent
	// save while one is already in flight.
	next, tickCmd := m.Update(saveMsg{gen: m.gen})
	m = next.(Model)
	if tickCmd != nil {
		t.Fatal("no second save should be issued while one is in flight")
	}

	// The first save completes, having written the older content "a".
	saved := saveCmd()
	if _, ok := saved.(savedMsg); !ok {
		t.Fatalf("expected savedMsg, got %T", saved)
	}
	next, drainCmd := m.Update(saved)
	m = next.(Model)

	// Completion must drain: because the buffer changed to "ab" during the
	// in-flight save, a follow-up save is issued for the latest content.
	if drainCmd == nil {
		t.Fatal("completion should drain the newer content with a follow-up save")
	}
	drainCmd() // executes the follow-up write

	got, _ := notes.Read(p)
	if got != "ab" {
		t.Fatalf("disk should converge to latest content, got %q, want %q", got, "ab")
	}
}
