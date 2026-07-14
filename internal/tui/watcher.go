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
