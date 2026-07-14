package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPath(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if code := run(dir, []string{"path"}, &out); code != 0 {
		t.Fatalf("run path exit = %d, want 0", code)
	}
	want := filepath.Join(dir, ".scratch.md")
	if strings.TrimSpace(out.String()) != want {
		t.Fatalf("run path = %q, want %q", out.String(), want)
	}
}

func TestRunPrint(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".scratch.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := run(dir, []string{"print"}, &out); code != 0 {
		t.Fatalf("run print exit = %d, want 0", code)
	}
	if out.String() != "hello\n" {
		t.Fatalf("run print = %q, want %q", out.String(), "hello\n")
	}
}

func TestRunAppend(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if code := run(dir, []string{"append", "a line"}, &out); code != 0 {
		t.Fatalf("run append exit = %d, want 0", code)
	}
	got, _ := os.ReadFile(filepath.Join(dir, ".scratch.md"))
	if string(got) != "a line\n" {
		t.Fatalf("after append = %q, want %q", got, "a line\n")
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var out bytes.Buffer
	if code := run(t.TempDir(), []string{"bogus"}, &out); code != 2 {
		t.Fatalf("run bogus exit = %d, want 2", code)
	}
}
