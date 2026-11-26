package scene

import (
	"testing"
	"time"

	"github.com/kalyskhairy/nakama/internal/theme"
)

// homeTheme gives two characters a work station of their own; the chair is last, as in the real pack.
func homeTheme() *theme.Theme {
	th := testTheme()
	st := func(id string, f ...string) theme.Station {
		return theme.Station{ID: id, Point: theme.Point{X: 0.6, Y: 0.5}, Kind: "work", Room: id, For: f, Node: "n"}
	}
	th.Stations = []theme.Station{st("gym", "fencer"), st("lib", "scholar", "Explore"), st("spare"), st("chair", "captain"),
		{ID: "sofa", Point: theme.Point{X: 0.2, Y: 0.5}, Kind: "rest", Room: "r", Node: "n"}}
	return th
}

func as(a Actor, char string) Actor { a.Character = char; return a }

func TestEveryCharacterWorksAtItsOwnPlace(t *testing.T) {
	w := NewWorld(homeTheme())
	w.Update([]Actor{as(lead("z", Working, t0.Add(time.Minute)), "fencer"), as(lead("l", Working, t0), "captain")}, t0)
	if station(t, w, "z") != "gym" || station(t, w, "l") != "chair" {
		t.Fatalf("z %s l %s", station(t, w, "z"), station(t, w, "l"))
	}
}

func TestSubagentSharesTheRoomOrGoesByType(t *testing.T) {
	w := NewWorld(homeTheme())
	w.Update([]Actor{as(lead("z", Working, t0), "fencer"), as(sub("s", "Explore", Working, t0), "fencer")}, t0)
	if station(t, w, "z") != "gym" || station(t, w, "s") != "lib" {
		t.Fatalf("z %s s %s", station(t, w, "z"), station(t, w, "s"))
	}
}

func TestOwnerTakesItsPlaceBack(t *testing.T) {
	w := NewWorld(homeTheme())
	squat := as(sub("s", "x", Working, t0), "scholar")
	w.Update([]Actor{as(sub("a", "x", Working, t0.Add(time.Minute)), "fencer"), squat}, t0)
	if station(t, w, "s") != "lib" {
		t.Fatalf("setup: s at %s", station(t, w, "s"))
	}
	w.Update([]Actor{as(sub("a", "x", Working, t0), "fencer"), squat, as(lead("r", Working, t0), "scholar")}, t0.Add(time.Second))
	if station(t, w, "r") != "lib" {
		t.Fatalf("scholar at %s", station(t, w, "r"))
	}
	if got := station(t, w, "s"); got == "lib" || got == "chair" {
		t.Fatalf("the evicted one moves to a spare place, not %s", got)
	}
}
