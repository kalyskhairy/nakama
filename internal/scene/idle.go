package scene

import (
	"math"
	"math/rand/v2"
	"sort"

	"github.com/kalyskhairy/nakama/internal/theme"
)

// The characters no session wears hang about the corridor: they are decoration, not agents.
const (
	maxStrolling = 3
	minPause     = 5.0 // seconds standing between two strolls
	maxPause     = 20.0
	strollPace   = 0.5 // of the speed of somebody with work to do
	elbowRoom    = 0.05
	idlePrefix   = "idle:"
)

func pause() float64 { return minPause + rand.Float64()*(maxPause-minPause) }

// spot is a random place on the corridor, away from where the others stand or head if it can be.
func (w *World) spot(me string) theme.Point {
	var p theme.Point
	for try := 0; try < 8; try++ {
		i := rand.IntN(len(w.th.Corridor) - 1)
		a, b := w.th.Nodes[w.th.Corridor[i]], w.th.Nodes[w.th.Corridor[i+1]]
		f := rand.Float64()
		p = theme.Point{X: a.X + (b.X-a.X)*f, Y: a.Y + (b.Y-a.Y)*f}
		clear := true
		for name, o := range w.idle {
			at := o.pos
			if len(o.path) > 0 {
				at = o.path[len(o.path)-1]
			}
			clear = clear && (name == me || w.dist(p, at) >= elbowRoom)
		}
		if clear {
			break
		}
	}
	return p
}

// muster keeps one stroller for every character that no lead wears. left holds where the characters
// of leads that just left were, so they walk back from there.
func (w *World) muster(worn map[string]bool, left map[string]theme.Point) {
	if len(w.th.Corridor) < 2 {
		return
	}
	for _, name := range w.th.SessionCharacters {
		switch {
		case worn[name]:
			delete(w.idle, name)
		case w.idle[name] == nil:
			if at, ok := left[name]; ok {
				to := w.spot(name)
				w.idle[name] = &body{pos: at, path: w.route(at, theme.Station{Point: to, Node: w.nearestNode(to)})}
			} else {
				w.idle[name] = &body{pos: w.spot(name), wait: rand.Float64() * maxPause}
			}
		}
	}
}

// stroll moves the off-duty crew: mostly standing, now and then to another spot.
func (w *World) stroll(dt float64) {
	names := make([]string, 0, len(w.idle))
	walking := 0
	for name, b := range w.idle {
		names = append(names, name)
		if len(b.path) > 0 {
			walking++
		}
	}
	sort.Strings(names)
	rand.Shuffle(len(names), func(i, j int) { names[i], names[j] = names[j], names[i] })
	for _, name := range names {
		b := w.idle[name]
		if len(b.path) == 0 {
			b.wait = math.Max(b.wait-dt, 0)
			if b.wait == 0 && walking < maxStrolling {
				b.path = []theme.Point{w.spot(name)} // along the corridor, which is straight between its nodes
				walking++
			}
			continue
		}
		w.step(b, walkSpeed*strollPace, dt)
		if len(b.path) == 0 {
			b.wait = pause()
		}
	}
}

func (w *World) strollers() []Placed {
	var out []Placed
	for name, b := range w.idle {
		p := Placed{ID: idlePrefix + name, Character: name, P: b.pos, Idle: true}
		if len(b.path) > 0 {
			p.Anim, p.Facing, p.Frame = Walk, b.facing, int(b.phase*walkFrames/stepsPerSec)%4
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
