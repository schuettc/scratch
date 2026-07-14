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
