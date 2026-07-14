package tui

import (
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
	gen         int  // debounce generation
	saving      bool // a write is currently in flight
	quitting    bool // ctrl+q/esc pressed; quit once the latest content is flushed
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

// triggerSave issues an atomic write of the current buffer, but only if no
// write is already in flight. Serializing writes this way prevents two
// concurrent notes.Write calls from racing on the same file — a stale one
// could otherwise land last and clobber newer content. When the in-flight
// write completes (savedMsg), the buffer is re-checked and another save is
// issued if it changed meanwhile, so the latest content converges to disk.
func (m Model) triggerSave() (Model, tea.Cmd) {
	if m.saving {
		return m, nil
	}
	m.saving = true
	return m, m.saveNow()
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
			m.quitting = true
			if m.saving {
				// A write is already in flight; quit once it (and any drain of
				// newer content) completes, so we never start a second racing
				// write that could land stale and lose the latest edits.
				return m, nil
			}
			if m.dirty {
				nm, cmd := m.triggerSave()
				return nm, cmd
			}
			return m, tea.Quit
		case "ctrl+s":
			nm, cmd := m.triggerSave()
			return nm, cmd
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
			nm, cmd := m.triggerSave()
			return nm, cmd
		}
		return m, nil

	case savedMsg:
		m.saving = false
		if msg.err != nil {
			m.saveErr = msg.err.Error()
			if m.quitting {
				// Don't trap the user if the file cannot be written.
				return m, tea.Quit
			}
			return m, nil
		}
		m.saveErr = ""
		m.lastWritten = msg.content
		m.dirty = m.textarea.Value() != msg.content
		// Our write is now the on-disk content, so any prior "changed on disk"
		// flag no longer applies. Clearing it here also suppresses the spurious
		// flag a self-write's fsnotify echo could raise before this savedMsg.
		m.diskChanged = false
		if m.dirty {
			// Buffer changed while this save was in flight — drain the latest.
			nm, cmd := m.triggerSave()
			return nm, cmd
		}
		if m.quitting {
			return m, tea.Quit
		}
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
