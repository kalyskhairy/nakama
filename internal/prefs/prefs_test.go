package prefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMissingFileIsFirstRun(t *testing.T) {
	f := Load(filepath.Join(t.TempDir(), "folders.json"))
	if !f.First() || f.Shown("/x") || !f.IsNew("/x") {
		t.Fatal("a missing file must act as a first run with nothing shown")
	}
}

func TestRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "folders.json")
	f := Load(p)
	f.Set("/a", true)
	f.Set("/b", true)
	f.Set("/b", false)
	f.MarkSeen([]string{"/a", "/b"})
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	g := Load(p)
	if g.First() || !g.Shown("/a") || g.Shown("/b") || g.IsNew("/a") || g.IsNew("/b") || !g.IsNew("/c") {
		t.Fatalf("%+v", g)
	}
}

func TestNewFolderIsHiddenAndNew(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.json")
	f := Load(p)
	f.Set("/a", true)
	f.MarkSeen([]string{"/a"})
	f.Save()
	g := Load(p)
	if g.Shown("/later") || !g.IsNew("/later") {
		t.Fatal("a folder that appears later must be hidden and new")
	}
}

func TestSetMarksSeen(t *testing.T) {
	f := Load(filepath.Join(t.TempDir(), "f.json"))
	f.Set("/made-with-n", true)
	if f.IsNew("/made-with-n") {
		t.Fatal("a folder the owner chose is not new")
	}
}

func TestCorruptFileActsAsFirstRunAndIsNotOverwrittenBeforeSave(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.json")
	os.WriteFile(p, []byte("{broken"), 0o644)
	f := Load(p)
	if !f.First() {
		t.Fatal("corrupt file must act as first run")
	}
	if b, _ := os.ReadFile(p); string(b) != "{broken" {
		t.Fatal("Load must not touch the file")
	}
}

func TestSaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	f := Load(filepath.Join(dir, "f.json"))
	f.Set("/a", true)
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 || ents[0].Name() != "f.json" {
		t.Fatalf("left over files: %v", ents)
	}
}
