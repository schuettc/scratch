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
