// Package notes is the file layer for the scratch scratchpad: path
// resolution, atomic reads/writes, append, and the reload decision.
package notes

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Path returns the scratchpad file path for a given working directory.
func Path(cwd string) string {
	return filepath.Join(cwd, ".scratch.md")
}

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
