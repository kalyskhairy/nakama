package app

import (
	"strings"
	"testing"

	"github.com/kalyskhairy/nakama/internal/input"
	"github.com/kalyskhairy/nakama/internal/monitor"
)

func TestClickOnACrewListNameOpensItsChat(t *testing.T) {
	subs := map[string][]monitor.SubInfo{"b": {{ID: "x", Type: "Explore", Description: "look", Info: monitor.Info{Status: monitor.Working}}}}
	m, _ := officeModel(t, map[string]monitor.Info{}, subs, "a", "b")
	m.Resize(200, 45)
	m.mode = input.Home
	m.Tick(t0)
	Frame(m, 200, 45, t0)
	l, _ := m.officeLayout(200, 45)
	lines := m.crewLines(l.crew.W, l.crew.H, t0)
	row := func(sub string) int {
		for i, s := range lines {
			if strings.Contains(s, sub) {
				return l.crew.Y + i
			}
		}
		t.Fatalf("no %q in %q", sub, lines)
		return 0
	}
	click := func(x, y int) { m.HandleEvent(input.Event{Kind: input.EvMouse, Press: true, X: x, Y: y}, t0) }

	click(2, l.crew.Y) // the header
	click(l.crew.W-3, row("└"))
	if m.Mode() != input.Home || m.selected != "" {
		t.Fatalf("header and card side do nothing: mode %v selected %q", m.Mode(), m.selected)
	}
	click(2, row("└")) // the subagent: its lead's chat
	if m.selected != "b" || m.Mode() != input.Chat || m.selectedID(t0) != "b" {
		t.Fatalf("selected %q mode %v chat %q", m.selected, m.Mode(), m.selectedID(t0))
	}
	Frame(m, 200, 45, t0)
	lines = m.crewLines(l.crew.W, l.crew.H, t0)
	first := -1
	for i, s := range lines {
		if strings.Contains(s, "[") {
			first = l.crew.Y + i
			break
		}
	}
	other := m.crewRows(t0)[0].ID
	click(2, first+1) // the title line of the first row, from inside a chat
	if m.selected != other || m.Mode() != input.Chat || m.selectedID(t0) != other {
		t.Fatalf("selected %q, chat %q, want %q", m.selected, m.selectedID(t0), other)
	}
}
