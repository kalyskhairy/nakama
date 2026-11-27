package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"alpha", "alpine", "beta", ".hidden"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	os.WriteFile(filepath.Join(root, "file.txt"), nil, 0o644)
	os.Symlink(filepath.Join(root, "beta"), filepath.Join(root, "link"))
	return root
}

func typed(p *NewPicker, s string) {
	for _, r := range s {
		p.Key(string(r))
	}
}

func TestTabCompletesAFolderPath(t *testing.T) {
	root := tree(t)
	for in, want := range map[string]string{
		root + "/b":   root + "/beta/", // one folder
		root + "/al":  root + "/alp",   // the part they share
		root + "/f":   root + "/f",     // a file is no place for a session
		root + "/.h":  root + "/.hidden/",
		root + "/li":  root + "/link/", // a link to a folder counts
		root + "/zz":  root + "/zz",
		root + "/x/y": root + "/x/y", // no such folder
	} {
		p := NewNewPicker(nil, []string{"personal", "work"}, nil)
		typed(p, in)
		p.Key("tab")
		if p.Query != want {
			t.Errorf("%q: got %q, want %q", in, p.Query, want)
		}
		if a := p.account(p.dir()); a != "personal" {
			t.Errorf("%q: Tab in a path changed the account to %s", in, a)
		}
	}
}

func TestTabListsTheChoicesWhenItCannotGoFurther(t *testing.T) {
	root := tree(t)
	p := NewNewPicker(nil, []string{"personal"}, nil)
	typed(p, root+"/a")
	p.Key("tab")
	p.Key("tab")
	got := strings.Join(p.Lines(100, 20), "\n")
	if p.Query != root+"/alp" || !strings.Contains(got, "alpha/") || !strings.Contains(got, "alpine/") || strings.Contains(got, ".hidden") {
		t.Fatalf("%q:\n%s", p.Query, got)
	}
	p.Key("h")
	if got := strings.Join(p.Lines(100, 20), "\n"); strings.Contains(got, "alpine/") {
		t.Fatal("typing clears the choices")
	}
}

func TestTabCompletesFromHome(t *testing.T) {
	root := tree(t)
	t.Setenv("HOME", root)
	p := NewNewPicker(nil, []string{"personal"}, nil)
	typed(p, "~")
	p.Key("tab")
	typed(p, "b")
	p.Key("tab")
	if p.Query != "~/beta/" {
		t.Fatalf("%q", p.Query)
	}
}

func TestShiftTabChangesTheAccountInAPath(t *testing.T) {
	p := NewNewPicker(nil, []string{"personal", "work"}, nil)
	typed(p, "/tmp")
	p.Key("shift-tab")
	if a := p.account(p.dir()); a != "work" {
		t.Fatalf("%s", a)
	}
	if !strings.Contains(strings.Join(p.Lines(120, 10), "\n"), "Tab completes") {
		t.Fatal("the hint must say what Tab does now")
	}
}
