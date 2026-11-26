package scene

import (
	"math"
	"testing"
	"time"
)

func TestAVisitorWalksToTheOtherAndBack(t *testing.T) {
	w := NewWorld(testTheme())
	a, b := lead("a", Working, t0.Add(time.Minute)), lead("b", Working, t0)
	run := func(from, secs int) {
		for i := from; i < from+secs*4; i++ {
			w.Update([]Actor{a, b}, t0.Add(time.Duration(i)*250*time.Millisecond))
		}
	}
	at := func(id string) Placed {
		for _, p := range w.Placed() {
			if p.ID == id {
				return p
			}
		}
		t.Fatalf("%s not placed", id)
		return Placed{}
	}
	run(0, 20)
	home := at("a").P
	a.Visit = "b"
	run(80, 20)
	if d := math.Abs(at("a").P.X - at("b").P.X); d > 0.06 {
		t.Fatalf("not beside the other: %v", d)
	}
	if at("a").Mark != 'm' || at("b").Mark != 'm' {
		t.Fatalf("no speech bubbles: %q %q", at("a").Mark, at("b").Mark)
	}
	a.Visit = ""
	run(160, 20)
	if at("a").P != home {
		t.Fatalf("did not go back: %v %v", at("a").P, home)
	}
}

func TestAFullContextShowsAMarkUnlessSomethingElseDoes(t *testing.T) {
	w := NewWorld(testTheme())
	a := lead("a", Working, t0)
	a.Full = true
	w.Update([]Actor{a}, t0)
	if m := w.Placed()[0].Mark; m != '!' {
		t.Fatalf("mark %q", m)
	}
	a.Status = Waiting
	w.Update([]Actor{a}, t0)
	if m := w.Placed()[0].Mark; m != '?' {
		t.Fatalf("waiting comes first: %q", m)
	}
}
