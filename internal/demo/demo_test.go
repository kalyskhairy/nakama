package demo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kalyskhairy/nakama/internal/office"
	"github.com/kalyskhairy/nakama/internal/theme"
)

func TestTheDemoThemeIsAWholeTheme(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir); err != nil {
		t.Fatal(err)
	}
	th, err := theme.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := office.NewRenderer(th); err != nil {
		t.Fatal(err)
	}
	if len(th.SessionCharacters) < 6 || len(th.Corridor) < 2 {
		t.Fatalf("cast %d corridor %d", len(th.SessionCharacters), len(th.Corridor))
	}
	for _, c := range th.SessionCharacters { // everybody has a place of its own
		own := false
		for _, s := range th.Stations {
			for _, f := range s.For {
				own = own || (f == c && s.Kind == "work")
			}
		}
		if !own {
			t.Errorf("%s has no work place", c)
		}
	}
	// a second write keeps what is there, so an owner's edits survive
	os.WriteFile(filepath.Join(dir, "theme.json"), []byte("mine"), 0o644)
	if err := Write(dir); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "theme.json")); string(b) != "mine" {
		t.Fatal("overwritten")
	}
}
