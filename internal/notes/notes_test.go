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
