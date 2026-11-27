package workspace

import (
	"strings"
	"testing"

	"github.com/kalyskhairy/nakama/internal/input"
)

func TestHelpListsEveryBindingAndMeaning(t *testing.T) {
	all := strings.Join(HelpLines(200), "\n")
	for _, b := range input.Bindings {
		if !strings.Contains(all, b.Keys) || !strings.Contains(all, b.Desc) {
			t.Errorf("help lacks %q / %q", b.Keys, b.Desc)
		}
	}
	for _, want := range []string{"waiting", "working", "idle", "done", "other terminal", "just active", "5h", "7d", "~/.config/nakama", "--offline", "Shift+drag"} {
		if !strings.Contains(all, want) {
			t.Errorf("help lacks %q", want)
		}
	}
}

func TestHelpWrapsToWidth(t *testing.T) {
	for _, l := range HelpLines(50) {
		if n := len([]rune(l)); n > 50 {
			t.Fatalf("line of %d columns: %q", n, l)
		}
	}
}

func TestScrollHelp(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e"}
	if got := ScrollHelp(lines, 0, 3); len(got) != 3 || got[0] != "a" {
		t.Fatalf("%v", got)
	}
	if got := ScrollHelp(lines, 99, 3); len(got) != 3 || got[0] != "c" {
		t.Fatalf("offset must clamp so the last page stays full: %v", got)
	}
	if got := ScrollHelp(lines, -4, 3); got[0] != "a" {
		t.Fatalf("%v", got)
	}
}
