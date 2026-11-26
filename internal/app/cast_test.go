package app

import (
	"testing"

	"github.com/kalyskhairy/nakama/internal/monitor"
	"github.com/kalyskhairy/nakama/internal/workspace"
)

func TestEachSessionKeepsItsOwnCharacterAndSubagentsShareIt(t *testing.T) {
	subs := map[string][]monitor.SubInfo{"a": {{ID: "x", Type: "Explore", Info: monitor.Info{Status: monitor.Working}}}}
	m, fm := officeModel(t, nil, subs, "a", "b", "c")
	m.office.Theme.SessionCharacters = []string{"one", "two", "three"}
	cast := func() map[string]string {
		m.Tick(t0)
		out := map[string]string{}
		for _, a := range m.Actors() {
			out[a.ID] = a.Character
		}
		return out
	}
	first := cast()
	if first["a"] == "" || first["a"] == first["b"] || first["b"] == first["c"] || first["a"] == first["c"] {
		t.Fatalf("three sessions, three characters: %v", first)
	}
	if first["sub:a:x"] != first["a"] {
		t.Fatalf("a subagent wears its session's character: %v", first)
	}
	fm.subs = nil
	if again := cast(); again["a"] != first["a"] || again["b"] != first["b"] || again["c"] != first["c"] {
		t.Fatalf("characters changed: %v -> %v", first, again)
	}
}

func TestTheFirstSessionIsAlwaysTheCaptainAndTheRestVary(t *testing.T) {
	m, _ := officeModel(t, nil, nil, "a")
	m.office.Theme.SessionCharacters = []string{"cap", "one", "two", "three"}
	rows := func(ids ...string) (out []workspace.ActiveRow) {
		for _, id := range ids {
			out = append(out, workspace.ActiveRow{ID: id})
		}
		return out
	}
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		m.cast = nil
		m.castCrew(rows("a", "b", "c", "d"), "c")
		if m.cast["c"] != "cap" {
			t.Fatalf("the first one is the captain: %v", m.cast)
		}
		got := map[string]bool{}
		for _, c := range m.cast {
			got[c] = true
		}
		if len(got) != 4 {
			t.Fatalf("everyone is used before anyone repeats: %v", m.cast)
		}
		seen[m.cast["a"]] = true
	}
	if len(seen) != 3 {
		t.Fatalf("the others are drawn at random: %v", seen)
	}
	m.castCrew(rows("a", "b", "d", "e"), "a") // the captain's session left
	if m.cast["e"] != "cap" {
		t.Fatalf("a newcomer takes over the free captain: %v", m.cast)
	}
}
