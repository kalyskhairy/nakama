package session

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMouseWantedFollowsEachModeNotACount(t *testing.T) {
	s := &Session{}
	s.setMode(ansi.ModeMouseNormal, true)
	s.setMode(ansi.ModeMouseNormal, true) // asked again, as after a resize
	s.setMode(ansi.ModeMouseNormal, false)
	if s.MouseWanted() {
		t.Fatal("turned off but still wanted")
	}
}

func TestEnvEntryWithoutValueRemovesIt(t *testing.T) {
	got := childEnv([]string{"A=1", "CLAUDE_CONFIG_DIR=/x"}, []string{"CLAUDE_CONFIG_DIR", "B=2"})
	if len(got) != 2 || got[0] != "A=1" || got[1] != "B=2" {
		t.Fatalf("%q", got)
	}
}
