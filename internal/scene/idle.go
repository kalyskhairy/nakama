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
	laneGap      = 0.4  // between two lanes, in character heights
	laneChange   = 0.03 // how far along the corridor a step to another lane takes, in background widths
)

func pause() float64 { return minPause + rand.Float64()*(maxPause-minPause) }

// lanePoint is the point a fraction f along corridor segment i, moved sideways into a lane.
// Lanes are numbered 0.. across the corridor, the middle one on the line itself.
func (w *World) lanePoint(i int, f float64, lane int) theme.Point {
	a, b := w.th.Nodes[w.th.Corridor[i]], w.th.Nodes[w.th.Corridor[i+1]]
	p := theme.Point{X: a.X + (b.X-a.X)*f, Y: a.Y + (b.Y-a.Y)*f}
	dx, dy := b.X-a.X, (b.Y-a.Y)*w.aspect // in background widths, so the sides are square
	l := math.Hypot(dx, dy)
	if l == 0 {
		return p
	}
	// sideways by a share of the character height (a fraction of the background height)
	k := float64(lane) - float64(w.th.CorridorLanes-1)/2
	off := k * laneGap * w.th.CharHeight
	return theme.Point{X: p.X - dy/l*off*w.aspect, Y: p.Y + dx/l*off}
}

// spot is a random place on the corridor, in a random lane, away from where the others stand or
// head if it can be.
func (w *World) spot(me string) (theme.Point, int) {
	var p theme.Point
	lane := 0
	for try := 0; try < 8; try++ {
		lane = rand.IntN(max(w.th.CorridorLanes, 1))
		p = w.lanePoint(rand.IntN(len(w.th.Corridor)-1), rand.Float64(), lane)
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
	return p, lane
}

// stroll path is the way to a spot: straight along the lane, after a short step across when the spot
// is in another lane.
func (w *World) strollPath(b *body, to theme.Point, lane int) []theme.Point {
	if lane == b.lane {
		return []theme.Point{to}
	}
	if w.dist(b.pos, to) <= laneChange*2 {
		return []theme.Point{to}
	}
	seg := w.segmentOf(b.pos)
	a, c := w.th.Nodes[w.th.Corridor[seg]], w.th.Nodes[w.th.Corridor[seg+1]]
	dx, dy := c.X-a.X, (c.Y-a.Y)*w.aspect // in background widths, so the sides are square
	l := math.Hypot(dx, dy)
	if l == 0 {
		return []theme.Point{to}
	}
	ux, uy := dx/l, dy/l
	step := laneChange // forward along the corridor, toward the spot
	if (to.X-b.pos.X)*ux+(to.Y-b.pos.Y)*w.aspect*uy < 0 {
		step = -step
	}
	off := float64(lane-b.lane) * laneGap * w.th.CharHeight // and across into the new lane
	across := theme.Point{X: b.pos.X + ux*step - uy*off*w.aspect, Y: b.pos.Y + uy*step/w.aspect + ux*off}
	return []theme.Point{across, to}
}

// segmentOf is the corridor segment nearest to a point.
func (w *World) segmentOf(p theme.Point) int {
	best, bestD := 0, math.Inf(1)
	for i := 0; i+1 < len(w.th.Corridor); i++ {
		a, b := w.th.Nodes[w.th.Corridor[i]], w.th.Nodes[w.th.Corridor[i+1]]
		if d := w.dist(p, a) + w.dist(p, b) - w.dist(a, b); d < bestD {
			best, bestD = i, d
		}
	}
	return best
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
			to, lane := w.spot(name)
			if at, ok := left[name]; ok {
				w.idle[name] = &body{pos: at, lane: lane, path: w.route(at, theme.Station{Point: to, Node: w.nearestNode(to)})}
			} else {
				w.idle[name] = &body{pos: to, lane: lane, wait: rand.Float64() * maxPause}
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
				to, lane := w.spot(name)
				b.path, b.lane = w.strollPath(b, to, lane), lane // along the corridor, straight between its nodes
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
