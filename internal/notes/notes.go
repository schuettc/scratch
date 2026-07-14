// Package notes is the file layer for the scratch scratchpad: path
// resolution, atomic reads/writes, append, and the reload decision.
package notes

import "path/filepath"

// Path returns the scratchpad file path for a given working directory.
func Path(cwd string) string {
	return filepath.Join(cwd, ".scratch.md")
}
