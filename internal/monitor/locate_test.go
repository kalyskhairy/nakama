package monitor

import (
	"os"
	"path/filepath"
	"testing"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLocateFindsFileInAnyRoot(t *testing.T) {
	r1, r2 := t.TempDir(), t.TempDir()
	want := filepath.Join(r2, "-x-y", "abc.jsonl")
	touch(t, want)
	if got := Locate([]string{r1, r2}, "abc"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got := Locate([]string{r1, r2}, "zzz"); got != "" {
		t.Fatalf("missing id gave %q", got)
	}
}

func TestLocateIgnoresOtherIDs(t *testing.T) {
	r := t.TempDir()
	touch(t, filepath.Join(r, "-x", "abcd.jsonl"))
	if got := Locate([]string{r}, "abc"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestRootsFindsClaudeDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".claude", "projects"), 0o755)
	os.MkdirAll(filepath.Join(home, ".claude-work", "projects"), 0o755)
	os.MkdirAll(filepath.Join(home, ".claude-foo"), 0o755) // no projects dir
	got := Roots()
	want := []string{filepath.Join(home, ".claude", "projects"), filepath.Join(home, ".claude-work", "projects")}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v want %v", got, want)
	}
}
