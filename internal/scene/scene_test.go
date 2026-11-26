package scene

import (
	"math"
	"testing"
	"time"

	"github.com/kalyskhairy/nakama/internal/theme"
)

var t0 = time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)

func testTheme() *theme.Theme {
	st := func(id, kind, room string, x float64, f ...string) theme.Station {
		return theme.Station{ID: id, Point: theme.Point{X: x, Y: 0.5}, Kind: kind, Room: room, For: f, Node: "n"}
	}
	return &theme.Theme{
		Characters: map[string]string{"captain": "c", "scholar": "s"},
		AgentTypes: map[string]string{"session": "captain", "Explore": "scholar", "default": "captain"},
		Nodes:      map[string]theme.Point{"e": {X: 0.0, Y: 0.5}, "n": {X: 0.5, Y: 0.5}},
		Edges:      [][2]string{{"e", "n"}},
		Entrance:   "e",
		Stations: []theme.Station{
			st("captain", "work", "deck", 0.9, "session"),
			st("deskA", "work", "open", 0.6),
			st("deskB", "work", "open", 0.65),
			st("lab", "work", "lib", 0.3, "Explore"),
			st("sofa1", "rest", "roomX", 0.2),
			st("sofa2", "rest", "roomY", 0.25),
			st("bunk", "rest", "roomX", 0.1),
		},
	}
}

func lead(id string, st Status, last time.Time) Actor {
	return Actor{ID: id, Type: "session", Title: id, Status: st, LastActive: last, Lead: true}
}
func sub(id, typ string, st Status, last time.Time) Actor {
	return Actor{ID: id, Type: typ, Title: id, Status: st, LastActive: last}
}

func station(t *testing.T, w *World, id string) string {
	t.Helper()
	s, ok := w.Target(id)
	if !ok {
		t.Fatalf("%s has no station", id)
	}
	return s
}

func TestMostRecentLeadGetsCaptainStation(t *testing.T) {
	w := NewWorld(testTheme())
	w.Update([]Actor{lead("a", Working, t0), lead("b", Working, t0.Add(time.Minute))}, t0.Add(time.Minute))
	if got := station(t, w, "b"); got != "captain" {
		t.Fatalf("b at %q", got)
	}
	if got := station(t, w, "a"); got == "captain" {
		t.Fatal("older lead must not share the captain station")
	}
}

func TestSubagentPrefersStationForItsType(t *testing.T) {
	w := NewWorld(testTheme())
	w.Update([]Actor{sub("s1", "Explore", Working, t0)}, t0)
	if got := station(t, w, "s1"); got != "lab" {
		t.Fatalf("Explore at %q", got)
	}
	w.Update([]Actor{sub("s1", "Explore", Working, t0), sub("s2", "Explore", Working, t0)}, t0)
	if got := station(t, w, "s2"); got == "lab" {
		t.Fatalf("second Explore must not share lab, got %q", got)
	}
}

func TestWorkingActiveWaitingGoToWorkStationWithMarks(t *testing.T) {
	w := NewWorld(testTheme())
	as := []Actor{sub("w", "x", Working, t0), sub("a", "x", Active, t0), sub("q", "x", Waiting, t0)}
	w.Update(as, t0)
	_, snaps := run(w, as, t0, 5) // newcomers walk in one at a time
	marks := map[string]rune{}
	for _, p := range snaps[len(snaps)-1] {
		marks[p.ID] = p.Mark
	}
	if len(marks) != 3 {
		t.Fatalf("only %d actors in the office after 5 s", len(marks))
	}
	for _, id := range []string{"w", "a", "q"} {
		s := station(t, w, id)
		if s == "sofa1" || s == "sofa2" || s == "bunk" {
			t.Errorf("%s sits at rest station %s", id, s)
		}
	}
	if marks["q"] != '?' || marks["w"] != 0 || marks["a"] != 0 {
		t.Fatalf("marks %v", marks)
	}
}

func TestIdleGoesToRestStation(t *testing.T) {
	w := NewWorld(testTheme())
	w.Update([]Actor{sub("i", "x", Idle, t0)}, t0)
	switch station(t, w, "i") {
	case "sofa1", "sofa2", "bunk":
	default:
		t.Fatal("idle actor not at a rest station")
	}
}

func TestExitedRestsDimThenLeaves(t *testing.T) {
	w := NewWorld(testTheme())
	a := sub("x", "x", Exited, t0)
	w.Update([]Actor{a}, t0.Add(599*time.Second))
	p := w.Placed()
	if len(p) != 1 || !p[0].Dim || p[0].Mark != '✓' {
		t.Fatalf("%+v", p)
	}
	w.Update([]Actor{a}, t0.Add(601*time.Second))
	if len(w.Placed()) != 0 {
		t.Fatal("exited actor should have left after 600 s")
	}
}

func TestOverflowIsNotDrawnAndNoStationIsShared(t *testing.T) {
	w := NewWorld(testTheme())
	var as []Actor
	for _, id := range []string{"a", "b", "c", "d", "e", "f"} { // 4 work stations only
		as = append(as, sub(id, "x", Working, t0))
	}
	w.Update(as, t0)
	seen := map[string]string{}
	for _, a := range as {
		s, ok := w.Target(a.ID)
		if !ok {
			continue // overflow: not drawn
		}
		if other, dup := seen[s]; dup {
			t.Fatalf("%s and %s share %s", other, a.ID, s)
		}
		seen[s] = a.ID
	}
	if len(seen) != 4 {
		t.Fatalf("%d actors got a station, want 4 (one per work station)", len(seen))
	}
	_, snaps := run(w, as, t0, 8)
	if n := len(snaps[len(snaps)-1]); n != 4 {
		t.Fatalf("%d drawn, want 4", n)
	}
}

func TestRestPlacesFillRoomByRoom(t *testing.T) {
	w := NewWorld(testTheme())
	w.Update([]Actor{sub("a", "x", Idle, t0), sub("b", "x", Idle, t0)}, t0)
	room := map[string]string{"sofa1": "roomX", "sofa2": "roomY", "bunk": "roomX"}
	ra, rb := room[station(t, w, "a")], room[station(t, w, "b")]
	if ra == rb {
		t.Fatalf("both idle actors in %s while roomY is empty", ra)
	}
}

func TestAssignmentIsStableWhileStatusUnchanged(t *testing.T) {
	w := NewWorld(testTheme())
	as := []Actor{sub("a", "x", Working, t0), sub("b", "x", Working, t0), sub("c", "x", Idle, t0)}
	w.Update(as, t0)
	first := map[string]string{}
	for _, a := range as {
		first[a.ID] = station(t, w, a.ID)
	}
	w.Update(append([]Actor{sub("z", "x", Working, t0)}, as...), t0.Add(time.Second))
	for _, a := range as {
		if got := station(t, w, a.ID); got != first[a.ID] {
			t.Errorf("%s moved from %s to %s", a.ID, first[a.ID], got)
		}
	}
}

func TestStatusClassChangeReassigns(t *testing.T) {
	w := NewWorld(testTheme())
	w.Update([]Actor{sub("a", "x", Working, t0)}, t0)
	w.Update([]Actor{sub("a", "x", Idle, t0)}, t0.Add(time.Second))
	switch station(t, w, "a") {
	case "sofa1", "sofa2", "bunk":
	default:
		t.Fatal("actor that went idle stayed at a work station")
	}
}

func TestUnknownTypeUsesDefaultCharacterAndEmptyInputIsFine(t *testing.T) {
	w := NewWorld(testTheme())
	w.Update(nil, t0)
	as := []Actor{sub("a", "SomethingNew", Working, t0), sub("b", "Explore", Working, t0)}
	w.Update(as, t0)
	_, snaps := run(w, as, t0, 3)
	chars := map[string]string{}
	for _, p := range snaps[len(snaps)-1] {
		chars[p.ID] = p.Character
	}
	if chars["a"] != "captain" || chars["b"] != "scholar" {
		t.Fatalf("%v", chars)
	}
}

// --- Task 4: movement ---

func walkTheme() *theme.Theme {
	return &theme.Theme{
		Characters: map[string]string{"c": "c"},
		AgentTypes: map[string]string{"default": "c"},
		Nodes:      map[string]theme.Point{"e": {X: 0, Y: 0.5}, "m": {X: 0.5, Y: 0.5}, "far": {X: 0.9, Y: 0.5}, "up": {X: 0.5, Y: 0.1}},
		Edges:      [][2]string{{"e", "m"}, {"m", "far"}, {"m", "up"}},
		Entrance:   "e",
		Stations: []theme.Station{
			{ID: "desk", Point: theme.Point{X: 0.9, Y: 0.5}, Kind: "work", Node: "far"},
			{ID: "bed", Point: theme.Point{X: 0.5, Y: 0.1}, Kind: "rest", Node: "up"},
		},
	}
}

// run advances the world by total seconds in 0.1 s steps and returns every placed snapshot.
func run(w *World, as []Actor, from time.Time, total float64) (time.Time, [][]Placed) {
	var snaps [][]Placed
	now := from
	for i := 0; i < int(total*10+0.5); i++ {
		now = now.Add(100 * time.Millisecond)
		w.Update(as, now)
		snaps = append(snaps, w.Placed())
	}
	return now, snaps
}

func TestPathShortestAndNilWhenDisconnected(t *testing.T) {
	th := walkTheme()
	got := Path(th.Nodes, th.Edges, "e", "up")
	if len(got) != 3 || got[0] != "e" || got[1] != "m" || got[2] != "up" {
		t.Fatalf("path %v", got)
	}
	if p := Path(th.Nodes, th.Edges, "e", "e"); len(p) != 1 {
		t.Fatalf("self path %v", p)
	}
	nodes := map[string]theme.Point{"a": {}, "b": {X: 1}}
	if p := Path(nodes, nil, "a", "b"); p != nil {
		t.Fatalf("disconnected gave %v", p)
	}
	if p := Path(th.Nodes, th.Edges, "e", "nope"); p != nil {
		t.Fatalf("unknown node gave %v", p)
	}
}

func TestActorWalksAlongGraphAtConstantSpeed(t *testing.T) {
	w := NewWorld(walkTheme())
	as := []Actor{sub("a", "x", Working, t0)}
	w.Update(as, t0)
	_, snaps := run(w, as, t0, 1.0)
	p := snaps[len(snaps)-1][0]
	if p.P.X < 0.11 || p.P.X > 0.13 || p.P.Y != 0.5 {
		t.Fatalf("after 1 s at %+v, want x about 0.12", p.P)
	}
	if p.Anim != Walk || p.Facing != 2 {
		t.Fatalf("anim %v facing %v", p.Anim, p.Facing)
	}
}

func TestFacingFollowsMovement(t *testing.T) {
	th := walkTheme()
	th.Entrance = "far" // now the desk is behind: walking left
	th.Stations[0].Point, th.Stations[0].Node = theme.Point{X: 0.0, Y: 0.5}, "e"
	w := NewWorld(th)
	as := []Actor{sub("a", "x", Working, t0)}
	w.Update(as, t0)
	_, snaps := run(w, as, t0, 0.5)
	if f := snaps[len(snaps)-1][0].Facing; f != 1 {
		t.Fatalf("facing %d, want 1 (left)", f)
	}
}

func TestArrivedActorStopsAndPlaysStationAnim(t *testing.T) {
	for _, c := range []struct {
		st   Status
		anim Anim
		mark rune
	}{{Working, Work, 0}, {Active, Stand, 0}, {Waiting, Stand, '?'}} {
		w := NewWorld(walkTheme())
		as := []Actor{sub("a", "x", c.st, t0)}
		w.Update(as, t0)
		_, snaps := run(w, as, t0, 12)
		p := snaps[len(snaps)-1][0]
		if p.P != (theme.Point{X: 0.9, Y: 0.5}) || p.Anim != c.anim || p.Mark != c.mark {
			t.Errorf("%v: %+v", c.st, p)
		}
	}
}

func TestIdleArrivesAndSleeps(t *testing.T) {
	w := NewWorld(walkTheme())
	as := []Actor{sub("a", "x", Idle, t0)}
	w.Update(as, t0)
	_, snaps := run(w, as, t0, 12)
	p := snaps[len(snaps)-1][0]
	if p.Anim != Sleep || p.Mark != 'z' || p.P != (theme.Point{X: 0.5, Y: 0.1}) {
		t.Fatalf("%+v", p)
	}
	if first := snaps[0][0]; first.Mark == 'z' {
		t.Fatal("must not sleep before arriving")
	}
}

func TestStatusChangeMidWalkRoutesFromCurrentPosition(t *testing.T) {
	w := NewWorld(walkTheme())
	working := []Actor{sub("a", "x", Working, t0)}
	w.Update(working, t0)
	now, snaps := run(w, working, t0, 3)
	prev := snaps[len(snaps)-1][0].P
	idle := []Actor{sub("a", "x", Idle, t0)}
	var worst float64
	for i := 0; i < 150; i++ {
		now = now.Add(100 * time.Millisecond)
		w.Update(idle, now)
		cur := w.Placed()[0].P
		d := hypot(cur.X-prev.X, cur.Y-prev.Y)
		worst = max(worst, d)
		prev = cur
	}
	if worst > 0.12*0.1+0.002 {
		t.Fatalf("jumped %.4f in one 0.1 s step (max %.4f)", worst, 0.12*0.1)
	}
	if p := w.Placed()[0]; p.P != (theme.Point{X: 0.5, Y: 0.1}) || p.Anim != Sleep {
		t.Fatalf("did not end at the bed: %+v", p)
	}
}

func TestLargeClockJumpIsClamped(t *testing.T) {
	w := NewWorld(walkTheme())
	as := []Actor{sub("a", "x", Working, t0)}
	w.Update(as, t0)
	w.Update(as, t0.Add(time.Hour))
	if x := w.Placed()[0].P.X; x > 0.12*0.5+0.002 {
		t.Fatalf("x = %.3f after a 1 h jump, want at most 0.06", x)
	}
	w.Update(as, t0.Add(-time.Hour)) // clock going backwards must not panic or move
}

func TestNewcomersEnterOneAtATime(t *testing.T) {
	w := NewWorld(walkTheme())
	as := []Actor{sub("a", "x", Working, t0), sub("b", "x", Idle, t0)}
	w.Update(as, t0)
	_, snaps := run(w, as, t0, 0.5)
	if n := len(snaps[len(snaps)-1]); n != 1 {
		t.Fatalf("%d placed after 0.5 s, want 1 (the second waits at the door)", n)
	}
	_, snaps = run(w, as, t0.Add(500*time.Millisecond), 1.5)
	last := snaps[len(snaps)-1]
	if len(last) != 2 {
		t.Fatalf("%d placed", len(last))
	}
	if hypot(last[0].P.X-last[1].P.X, last[0].P.Y-last[1].P.Y) < 0.05 {
		t.Fatalf("walking in lock-step: %+v %+v", last[0].P, last[1].P)
	}
}

func hypot(a, b float64) float64 { return math.Hypot(a, b) }

// --- review fixes ---

func TestLeadKeepsAPlaceWhenSubagentsFillTheRestStations(t *testing.T) {
	w := NewWorld(testTheme()) // three rest stations
	as := []Actor{lead("L", Idle, t0)}
	for _, id := range []string{"a", "b", "c"} { // ids sort before "L" would not matter: leads must win
		as = append(as, sub(id, "general-purpose", Idle, t0))
	}
	w.Update(as[1:], t0)              // the subagents settle first and take every rest station
	w.Update(as, t0.Add(time.Second)) // now the lead goes idle too
	if _, ok := w.Target("L"); !ok {
		t.Fatal("the lead has no place although subagents hold every rest station")
	}
	n := 0
	for _, a := range as {
		if _, ok := w.Target(a.ID); ok {
			n++
		}
	}
	if n != 3 {
		t.Fatalf("%d actors placed, want exactly the 3 rest stations used", n)
	}
}

func TestLeadKeepsAWorkPlaceWhenSubagentsFillTheWorkStations(t *testing.T) {
	w := NewWorld(testTheme()) // four work stations
	var subs []Actor
	for _, id := range []string{"a", "b", "c", "d"} {
		subs = append(subs, sub(id, "x", Working, t0))
	}
	w.Update(subs, t0)
	w.Update(append([]Actor{lead("L", Working, t0)}, subs...), t0.Add(time.Second))
	if _, ok := w.Target("L"); !ok {
		t.Fatal("the lead is not drawn although it is working")
	}
}

func TestCaptainChairFollowsTheMostRecentLead(t *testing.T) {
	cap := func(w *World, id string) bool { s, _ := w.Target(id); return s == "captain" }

	// the holder goes idle: the chair passes to the working lead
	w := NewWorld(testTheme())
	w.Update([]Actor{lead("A", Working, t0), lead("B", Idle, t0.Add(-time.Hour))}, t0)
	if !cap(w, "A") {
		t.Fatal("A should hold the chair")
	}
	w.Update([]Actor{lead("A", Idle, t0), lead("B", Working, t0.Add(time.Minute))}, t0.Add(time.Minute))
	if !cap(w, "B") {
		s, _ := w.Target("B")
		t.Fatalf("B works but sits at %q while A is idle", s)
	}

	// the holder is still working but clearly older: the more recent lead takes the chair, the old holder takes its desk
	w = NewWorld(testTheme())
	w.Update([]Actor{lead("A", Working, t0), lead("B", Working, t0.Add(-time.Minute))}, t0)
	w.Update([]Actor{lead("A", Working, t0), lead("B", Working, t0.Add(2*time.Minute))}, t0.Add(2*time.Minute))
	if !cap(w, "B") || cap(w, "A") {
		a, _ := w.Target("A")
		t.Fatalf("expected B in the chair and A elsewhere, A at %q", a)
	}
	if _, ok := w.Target("A"); !ok {
		t.Fatal("the old holder lost its place instead of swapping")
	}

	// two leads active within 30 s of each other must not trade seats back and forth
	w = NewWorld(testTheme())
	w.Update([]Actor{lead("A", Working, t0), lead("B", Working, t0.Add(-time.Minute))}, t0)
	w.Update([]Actor{lead("A", Working, t0), lead("B", Working, t0.Add(10*time.Second))}, t0.Add(10*time.Second))
	if !cap(w, "A") {
		t.Fatal("seats swapped for a 10 s difference")
	}
}

func TestActorCharacterOverridesAgentType(t *testing.T) {
	w := NewWorld(testTheme())
	a := lead("a", Working, t0)
	a.Character = "scholar"
	w.Update([]Actor{a}, t0)
	if p := w.Placed(); len(p) != 1 || p[0].Character != "scholar" {
		t.Fatalf("%+v", p)
	}
}
