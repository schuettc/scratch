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
	colText     = lipgloss.Color("#cdd6f4")
	colSurface1 = lipgloss.Color("#45475a")

	// Title bar (a small filled bar): "scratch · <workspace> ●".
	titleLabelStyle = lipgloss.NewStyle().Background(colSurface1).Foreground(colMauve).Bold(true)
	titleInfoStyle  = lipgloss.NewStyle().Background(colSurface1).Foreground(colText)
	titleDotClean   = lipgloss.NewStyle().Background(colSurface1).Foreground(colSubtext0)
	titleDotDirty   = lipgloss.NewStyle().Background(colSurface1).Foreground(colGreen)
	titleBarStyle   = lipgloss.NewStyle().Background(colSurface1)

	// Status line (data, not command hints).
	statusStyle = lipgloss.NewStyle().Foreground(colSubtext0)
	flagStyle   = lipgloss.NewStyle().Foreground(colRed)
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
	gen         int       // debounce generation
	saving      bool      // a write is currently in flight
	quitting    bool      // ctrl+q/esc pressed; quit once the latest content is flushed
	width       int       // last known terminal width (for the title bar)
	savedAt     time.Time // time of the last successful save (zero = never)

	confirmingClear bool // ctrl+x armed a "clear all? y/n" confirmation
}

// New builds a model with the file's current contents loaded.
func New(path string) Model {
	content, _ := notes.Read(path)

	ta := textarea.New()
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
		m.width = msg.Width
		m.textarea.SetWidth(msg.Width)
		h := msg.Height - 2 // title bar + status line
		if h < 1 {
			h = 1
		}
		m.textarea.SetHeight(h)
		return m, nil

	case tea.KeyMsg:
		// A pending "clear all?" confirmation consumes the next keypress:
		// "y" wipes the buffer (and saves), anything else cancels.
		if m.confirmingClear {
			m.confirmingClear = false
			if msg.String() == "y" {
				m.textarea.SetValue("")
				m.dirty = true
				m.gen++
				nm, cmd := m.triggerSave()
				return nm, cmd
			}
			return m, nil
		}
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
		case "ctrl+x":
			// Arm a confirmation; the next keypress decides (see above).
			m.confirmingClear = true
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
		m.savedAt = time.Now()
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

	// Title bar: "scratch · <workspace> ●" on a small filled bar.
	dot := titleDotClean.Render("○")
	if m.dirty {
		dot = titleDotDirty.Render("●")
	}
	title := titleLabelStyle.Render(" scratch ") +
		titleInfoStyle.Render("· "+name+" ") + dot
	width := m.width
	if width < lipgloss.Width(title) {
		width = lipgloss.Width(title)
	}
	titleBar := titleBarStyle.Width(width).Render(title)

	// Status line: data, not command hints.
	var status string
	switch {
	case m.confirmingClear:
		status = flagStyle.Render("clear all? y/n")
	case m.saveErr != "":
		status = errStyle.Render("save error: " + m.saveErr)
	case m.diskChanged:
		status = flagStyle.Render("● changed on disk")
	case !m.savedAt.IsZero():
		status = statusStyle.Render("saved " + m.savedAt.Format("15:04"))
	default:
		status = statusStyle.Render("not saved yet")
	}

	return lipgloss.JoinVertical(lipgloss.Left, titleBar, m.textarea.View(), status)
}
