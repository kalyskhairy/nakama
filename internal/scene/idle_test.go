package scene

import (
	"math"
	"testing"
	"time"

	"github.com/kalyskhairy/nakama/internal/theme"
)

func corridorTheme() *theme.Theme {
	th := testTheme()
	th.Characters = map[string]string{"captain": "c", "scholar": "s", "cook": "k", "doc": "d", "smith": "m"}
	th.SessionCharacters = []string{"captain", "scholar", "cook", "doc", "smith"}
	th.Nodes["w"] = theme.Point{X: 0.9, Y: 0.5}
	th.Edges = append(th.Edges, [2]string{"n", "w"})
	th.Corridor = []string{"e", "n", "w"}
	return th
}

func idlers(w *World) map[string]Placed {
	out := map[string]Placed{}
	for _, p := range w.Placed() {
		if p.Idle {
			out[p.Character] = p
		}
	}
	return out
}

func TestTheOffDutyCrewHangsAboutTheCorridor(t *testing.T) {
	w := NewWorld(corridorTheme())
	seen := map[string]map[float64]bool{}
	for i := 0; i < 4*300; i++ {
		w.Update(nil, t0.Add(time.Duration(i)*250*time.Millisecond))
		walking := 0
		for name, p := range idlers(w) {
			if math.Abs(p.P.Y-0.5) > 1e-9 || p.P.X < 0 || p.P.X > 0.9 {
				t.Fatalf("%s left the corridor: %v", name, p.P)
			}
			if p.Anim == Walk {
				walking++
			}
			if seen[name] == nil {
				seen[name] = map[float64]bool{}
			}
			seen[name][p.P.X] = true
		}
		if walking > maxStrolling {
			t.Fatalf("%d walk at once", walking)
		}
	}
	if len(seen) != 5 {
		t.Fatalf("all five are about: %d", len(seen))
	}
	for name, xs := range seen {
		if len(xs) < 2 {
			t.Errorf("%s never moved", name)
		}
	}
}

func TestACharacterOnDutyLeavesTheCorridorAndComesBack(t *testing.T) {
	w := NewWorld(corridorTheme())
	w.Update(nil, t0)
	from := idlers(w)["scholar"].P
	a := lead("a", Working, t0)
	a.Character = "scholar"
	w.Update([]Actor{a}, t0)
	if _, ok := idlers(w)["scholar"]; ok {
		t.Fatal("not twice")
	}
	ps := w.Placed()
	var p Placed
	for _, q := range ps {
		if q.ID == "a" {
			p = q
		}
	}
	if p.ID != "a" || w.dist(p.P, from) > 0.01 {
		t.Fatalf("starts where it was standing: %v, was %v", p.P, from)
	}
	for i := 1; i < 80; i++ {
		w.Update([]Actor{a}, t0.Add(time.Duration(i)*250*time.Millisecond))
	}
	atDesk := w.bodies["a"].pos
	w.Update(nil, t0.Add(20*time.Second))
	back, ok := idlers(w)["scholar"]
	if !ok || w.dist(back.P, atDesk) > 0.04 { // it has already taken its first step
		t.Fatalf("walks back from its desk: %v %v, desk %v", ok, back.P, atDesk)
	}
}

func TestNoCorridorNoStrollers(t *testing.T) {
	th := corridorTheme()
	th.Corridor = nil
	w := NewWorld(th)
	w.Update(nil, t0)
	if len(w.Placed()) != 0 {
		t.Fatal("a theme without a corridor stays as it was")
	}
}

func TestTheOffDutyCrewUsesEveryLaneAndCrossesBetweenThem(t *testing.T) {
	th := corridorTheme()
	th.CharHeight, th.CorridorLanes = 0.1, 3
	w := NewWorld(th)
	gap := laneGap * th.CharHeight
	lanes := map[float64]bool{}
	crossed := map[string]bool{}
	last := map[string]float64{}
	for i := 0; i < 4*600; i++ {
		w.Update(nil, t0.Add(time.Duration(i)*250*time.Millisecond))
		for name, p := range idlers(w) {
			if p.P.Y < 0.5-gap-1e-9 || p.P.Y > 0.5+gap+1e-9 {
				t.Fatalf("%s left the corridor: %v", name, p.P)
			}
			if p.Anim == Walk {
				continue
			}
			y := math.Round((p.P.Y-0.5)/gap*1000) / 1000
			if y != -1 && y != 0 && y != 1 {
				t.Fatalf("%s stands between lanes: %v", name, p.P)
			}
			lanes[y] = true
			if l, ok := last[name]; ok && l != y {
				crossed[name] = true
			}
			last[name] = y
		}
	}
	if len(lanes) != 3 || len(crossed) == 0 {
		t.Fatalf("lanes %v, crossed %v", lanes, crossed)
	}
}
