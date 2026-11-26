package workspace

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kalyskhairy/nakama/internal/index"
	"github.com/kalyskhairy/nakama/internal/prefs"
)

var folders = []index.Folder{{Cwd: "/home/k/nakama", Count: 3, Latest: now}, {Cwd: "/home/k/webshop", Count: 1, Latest: now.Add(-time.Hour)}}

func TestFilterPickerToggleAndApply(t *testing.T) {
	f := prefs.Load(filepath.Join(t.TempDir(), "f.json"))
	p := NewFilterPicker(folders, f)
	p.Key(" ") // toggles the first folder on
	p.Key("down")
	p.Key("j")
	if done := p.Key("enter"); !done {
		t.Fatal("enter must finish")
	}
	p.Apply(f)
	if !f.Shown("/home/k/nakama") || f.Shown("/home/k/webshop") || f.IsNew("/home/k/nakama") || f.IsNew("/home/k/webshop") {
		t.Fatal("applied state wrong")
	}
}

func TestFilterPickerSavedOnlyAfterEnter(t *testing.T) {
	f := prefs.Load(filepath.Join(t.TempDir(), "f.json"))
	p := NewFilterPicker(folders, f)
	if p.Saved() {
		t.Fatal("saved before finishing")
	}
	p.Key("esc")
	if p.Saved() {
		t.Fatal("esc must not count as saved")
	}
	q := NewFilterPicker(folders, f)
	q.Key("enter")
	if !q.Saved() {
		t.Fatal("enter must count as saved")
	}
}

func TestFilterPickerEscChangesNothing(t *testing.T) {
	f := prefs.Load(filepath.Join(t.TempDir(), "f.json"))
	p := NewFilterPicker(folders, f)
	p.Key(" ")
	if !p.Key("esc") {
		t.Fatal("esc must finish")
	}
	p.Apply(f)
	if f.Shown("/home/k/nakama") || !f.IsNew("/home/k/nakama") {
		t.Fatal("esc must not change the filter")
	}
}

func TestFilterPickerAllAndLines(t *testing.T) {
	f := prefs.Load(filepath.Join(t.TempDir(), "f.json"))
	p := NewFilterPicker(folders, f)
	p.Key("a")
	p.Key("enter")
	p.Apply(f)
	if !f.Shown("/home/k/nakama") || !f.Shown("/home/k/webshop") {
		t.Fatal("a must show all")
	}
	if got := strings.Join(p.Lines(60, 10), "\n"); !strings.Contains(got, "[x]") || !strings.Contains(got, "nakama (3)") {
		t.Fatalf("%s", got)
	}
}

func TestNewPickerPathOverridesList(t *testing.T) {
	p := NewNewPicker(folders, []string{"personal", "work"}, func(string) string { return "personal" })
	for _, r := range "~/x" {
		p.Key(string(r))
	}
	if !p.Key("enter") {
		t.Fatal("enter must finish")
	}
	if dir, acct, ok := p.Result(); !ok || dir != "~/x" || acct != "personal" {
		t.Fatalf("%q %q %v", dir, acct, ok)
	}
}

func TestNewPickerPicksFolderFromList(t *testing.T) {
	p := NewNewPicker(folders, []string{"personal", "work"}, func(cwd string) string {
		if strings.HasSuffix(cwd, "webshop") {
			return "work"
		}
		return "personal"
	})
	p.Key("down")
	p.Key("enter")
	if dir, acct, ok := p.Result(); !ok || dir != "/home/k/webshop" || acct != "work" {
		t.Fatalf("%q %q %v", dir, acct, ok)
	}
}

func TestNewPickerQueryFiltersList(t *testing.T) {
	p := NewNewPicker(folders, []string{"personal"}, nil)
	for _, r := range "shop" {
		p.Key(string(r))
	}
	p.Key("enter")
	if dir, _, ok := p.Result(); !ok || dir != "/home/k/webshop" {
		t.Fatalf("%q %v", dir, ok)
	}
}

func TestNewPickerTabCyclesAccount(t *testing.T) {
	p := NewNewPicker(folders, []string{"personal", "work"}, func(string) string { return "personal" })
	p.Key("tab")
	p.Key("enter")
	if _, acct, _ := p.Result(); acct != "work" {
		t.Fatal(acct)
	}
	p = NewNewPicker(folders, []string{"personal", "work"}, func(string) string { return "personal" })
	p.Key("tab")
	p.Key("tab")
	p.Key("enter")
	if _, acct, _ := p.Result(); acct != "personal" {
		t.Fatal(acct)
	}
}

func TestNewPickerEmptyListEmptyQueryNotOk(t *testing.T) {
	p := NewNewPicker(nil, []string{"personal"}, nil)
	p.Key("enter")
	if _, _, ok := p.Result(); ok {
		t.Fatal("nothing to start in")
	}
	p = NewNewPicker(folders, nil, nil)
	p.Key("esc")
	if _, _, ok := p.Result(); ok {
		t.Fatal("esc must not start anything")
	}
}

func TestNewPickerBackspace(t *testing.T) {
	p := NewNewPicker(folders, nil, nil)
	p.Key("é")
	p.Key("backspace")
	if p.Query != "" {
		t.Fatalf("%q", p.Query)
	}
}

func TestFilterPickerEnterWithNothingCheckedTakesTheHighlighted(t *testing.T) {
	f := prefs.Load(filepath.Join(t.TempDir(), "f.json"))
	p := NewFilterPicker(folders, f)
	p.Key("down")
	p.Key("enter")
	p.Apply(f)
	if f.Shown("/home/k/nakama") || !f.Shown("/home/k/webshop") {
		t.Fatal("Enter on a highlighted folder with nothing checked must show that folder")
	}
}
