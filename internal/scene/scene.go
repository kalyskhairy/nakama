// Package scene decides where each agent stands in the office and how it moves.
// It knows nothing about pixels: it works in fractions of the background.
package scene

import (
	"math"
	"sort"
	"time"

	"github.com/kalyskhairy/nakama/internal/theme"
)

type Status int

const (
	Idle Status = iota
	Active
	Working
	Waiting
	Exited
)

type Actor struct {
	ID, Type, Title string // Type: "session" for a lead, else the subagent's agentType
	Status          Status
	LastActive      time.Time
	Lead            bool
	Character       string // empty = chosen by Type
	Visit           string // id of an actor it is talking to: it walks there, and both get a bubble
}

type Anim int

const (
	Stand Anim = iota
	Walk
	Sleep
	Work
)

type Placed struct {
	ID        string
	Character string
	P         theme.Point
	Facing    int // 0 down, 1 left, 2 right, 3 up
	Anim      Anim
	Frame     int
	Mark      rune // '?' waiting, '✓' exited, 'z' asleep, 0 none
	Dim       bool
	Sub       bool // a subagent, drawn as a shadow
}

const (
	leaveAfter   = 600 * time.Second
	walkSpeed    = 0.12 // background widths per second
	stepsPerSec  = 2.5
	visitGap     = 0.04
	walkFrames   = 8.0 // walking frames per second: the legs must keep up with the ground covered
	maxStep      = 500 * time.Millisecond
	entryStagger = 1.0 // seconds between newcomers walking in
)

// body is where an actor physically is and where it is heading.
type body struct {
	pos     theme.Point
	path    []theme.Point
	station string // the station path leads to
	facing  int
	phase   float64
	wait    float64 // seconds before it walks in
}

type World struct {
	th      *theme.Theme
	assign  map[string]string // actor id -> station id
	actors  map[string]Actor
	station map[string]theme.Station
	bodies  map[string]*body
	lastNow time.Time
	aspect  float64 // background height / width, so distances are the same in both axes
}

func NewWorld(t *theme.Theme) *World {
	w := &World{th: t, assign: map[string]string{}, actors: map[string]Actor{}, station: map[string]theme.Station{},
		bodies: map[string]*body{}, aspect: 1}
	if t.Background != nil {
		if b := t.Background.Bounds(); b.Dx() > 0 {
			w.aspect = float64(b.Dy()) / float64(b.Dx())
		}
	}
	for _, s := range t.Stations {
		w.station[s.ID] = s
	}
	return w
}

func isWork(s Status) bool { return s == Working || s == Active || s == Waiting }

func suits(s theme.Station, typ string) bool {
	for _, f := range s.For {
		if f == typ {
			return true
		}
	}
	return false
}

// Update takes the current agents and fixes where each one belongs. An agent that is still
// in the same kind of state keeps its station; agents that do not fit are not drawn.
func (w *World) Update(actors []Actor, now time.Time) {
	present := map[string]Actor{}
	for _, a := range actors {
		if a.Status == Exited && !a.LastActive.IsZero() && now.Sub(a.LastActive) > leaveAfter {
			continue // walked out of the office
		}
		present[a.ID] = a
	}
	w.actors = present
	for id, st := range w.assign {
		a, ok := present[id]
		if !ok || (w.station[st].Kind == "work") != isWork(a.Status) {
			delete(w.assign, id)
		}
	}
	w.sendHome(present)
	taken := map[string]bool{}
	roomLoad := map[string]int{}
	for _, st := range w.assign {
		taken[st] = true
		roomLoad[w.station[st].Room]++
	}

	var work, rest []Actor
	for _, a := range present {
		if _, ok := w.assign[a.ID]; ok {
			continue
		}
		if isWork(a.Status) {
			work = append(work, a)
		} else {
			rest = append(rest, a)
		}
	}
	byPriority := func(list []Actor) {
		sort.Slice(list, func(i, j int) bool {
			a, b := list[i], list[j]
			if a.Lead != b.Lead {
				return a.Lead
			}
			if !a.LastActive.Equal(b.LastActive) {
				return a.LastActive.After(b.LastActive)
			}
			return a.ID < b.ID
		})
	}
	byPriority(work)
	byPriority(rest)

	free := func(kind string, pass func(theme.Station) bool) string {
		for _, s := range w.th.Stations {
			if s.Kind == kind && !taken[s.ID] && pass(s) {
				return s.ID
			}
		}
		return ""
	}
	claim := func(a Actor, id string) {
		w.assign[a.ID], taken[id] = id, true
		roomLoad[w.station[id].Room]++
	}
	// steal frees a station of this kind held by a subagent so that a lead is never left out.
	steal := func(kind string) string {
		var victims []string
		for id, st := range w.assign {
			if w.station[st].Kind == kind && !present[id].Lead {
				victims = append(victims, id)
			}
		}
		if len(victims) == 0 {
			return ""
		}
		sort.Strings(victims)
		v := victims[len(victims)-1]
		st := w.assign[v]
		delete(w.assign, v)
		taken[st] = false
		roomLoad[w.station[st].Room]--
		return st
	}
	for _, a := range work {
		pick := ""
		for _, pass := range []func(theme.Station) bool{
			func(s theme.Station) bool { return suits(s, a.Character) },
			func(s theme.Station) bool { return suits(s, a.Type) },
			func(s theme.Station) bool { return len(s.For) == 0 },
			func(s theme.Station) bool { return true },
		} {
			if pick = free("work", pass); pick != "" {
				break
			}
		}
		if pick == "" && a.Lead {
			pick = steal("work")
		}
		if pick != "" {
			claim(a, pick)
		}
	}
	for _, a := range rest {
		pick, best := "", 1<<30
		for _, s := range w.th.Stations {
			if s.Kind != "rest" || taken[s.ID] {
				continue
			}
			if l := roomLoad[s.Room]; l < best {
				pick, best = s.ID, l
			}
		}
		if pick == "" && a.Lead {
			pick = steal("rest")
		}
		if pick != "" {
			claim(a, pick)
		}
	}
	w.seatCaptain(present)
	w.move(now)
}

// sendHome seats every working lead at a station made for its character, moving out whoever sits
// there unless that one is a lead of the same character.
func (w *World) sendHome(present map[string]Actor) {
	var leads []string
	for id, a := range present {
		if a.Lead && a.Character != "" && isWork(a.Status) {
			leads = append(leads, id)
		}
	}
	sort.Strings(leads)
	for _, id := range leads {
		a := present[id]
		if suits(w.station[w.assign[id]], a.Character) {
			continue
		}
	stations:
		for _, s := range w.th.Stations {
			if s.Kind != "work" || !suits(s, a.Character) {
				continue
			}
			for other, st := range w.assign {
				if st != s.ID {
					continue
				}
				if o := present[other]; o.Lead && suits(s, o.Character) {
					continue stations
				}
				delete(w.assign, other)
			}
			w.assign[id] = s.ID
			break
		}
	}
}

const captainSwapAfter = 30 * time.Second

// seatCaptain gives the captain's chair (the first work station meant for "session") to the most
// recently active working lead. A holder that is a lead and nearly as recent keeps it, so two busy
// leads do not trade seats all the time.
func (w *World) seatCaptain(present map[string]Actor) {
	chair := ""
	for _, s := range w.th.Stations {
		if s.Kind == "work" && suits(s, "session") {
			chair = s.ID
			break
		}
	}
	if chair == "" {
		return
	}
	var best *Actor
	for id := range present {
		a := present[id]
		if a.Lead && isWork(a.Status) && (best == nil || a.LastActive.After(best.LastActive) || (a.LastActive.Equal(best.LastActive) && a.ID < best.ID)) {
			best = &a
		}
	}
	if best == nil {
		return
	}
	holder := ""
	for id, st := range w.assign {
		if st == chair {
			holder = id
		}
	}
	if holder == best.ID {
		return
	}
	if h, ok := present[holder]; ok && h.Lead && best.LastActive.Sub(h.LastActive) <= captainSwapAfter {
		return
	}
	old := w.assign[best.ID] // may be "" when the lead had no place
	if holder != "" {
		if old != "" {
			w.assign[holder] = old
		} else {
			delete(w.assign, holder)
		}
	}
	w.assign[best.ID] = chair
}

func (w *World) dist(a, b theme.Point) float64 { return math.Hypot(a.X-b.X, (a.Y-b.Y)*w.aspect) }

// route lists the points to walk from pos to a station: to the nearest node, along the
// graph, then to the station itself.
func (w *World) route(pos theme.Point, st theme.Station) []theme.Point {
	from, best := "", math.Inf(1)
	for id, n := range w.th.Nodes {
		if d := w.dist(pos, n); d < best || (d == best && id < from) {
			from, best = id, d
		}
	}
	var pts []theme.Point
	for _, id := range Path(w.th.Nodes, w.th.Edges, from, st.Node) {
		pts = append(pts, w.th.Nodes[id])
	}
	return append(pts, st.Point)
}

func (w *World) move(now time.Time) {
	dt := 0.0
	if !w.lastNow.IsZero() {
		dt = math.Min(math.Max(now.Sub(w.lastNow).Seconds(), 0), maxStep.Seconds())
	}
	w.lastNow = now
	for id := range w.bodies {
		if _, ok := w.assign[id]; !ok {
			delete(w.bodies, id)
		}
	}
	var newcomers []string
	for id := range w.assign {
		if w.bodies[id] == nil {
			newcomers = append(newcomers, id)
		}
	}
	sort.Strings(newcomers)
	for i, id := range newcomers {
		w.bodies[id] = &body{pos: w.th.Nodes[w.th.Entrance], wait: float64(i) * entryStagger}
	}
	for id, b := range w.bodies {
		st := w.assign[id]
		goal, dest := st, w.station[st]
		if v := w.actors[id].Visit; v != id && w.assign[v] != "" {
			dest = w.station[w.assign[v]]
			dest.X = math.Min(dest.X+visitGap, 1) // beside, not on top
			goal = "visit:" + w.assign[v]
		}
		if b.station != goal {
			b.station, b.path = goal, w.route(b.pos, dest)
		}
		if b.wait > 0 {
			b.wait = math.Max(b.wait-dt, 0)
			continue
		}
		remaining := walkSpeed * math.Max(w.aspect, 1) * dt // widths are short on an upright ship
		moved := false
		for remaining > 0 && len(b.path) > 0 {
			next := b.path[0]
			d := w.dist(b.pos, next)
			if dx, dy := next.X-b.pos.X, next.Y-b.pos.Y; d > 0 {
				if math.Abs(dx) >= math.Abs(dy*w.aspect) {
					b.facing = map[bool]int{true: 2, false: 1}[dx > 0]
				} else {
					b.facing = map[bool]int{true: 0, false: 3}[dy > 0]
				}
			}
			if d <= remaining {
				b.pos, b.path = next, b.path[1:]
				remaining -= d
			} else {
				f := remaining / d
				b.pos = theme.Point{X: b.pos.X + (next.X-b.pos.X)*f, Y: b.pos.Y + (next.Y-b.pos.Y)*f}
				remaining = 0
			}
			moved = true
		}
		if len(b.path) == 0 && moved {
			b.facing = 0 // arrived: face the viewer
		}
		b.phase += dt * stepsPerSec
	}
}

// Target returns the station an agent is assigned to.
func (w *World) Target(id string) (string, bool) {
	s, ok := w.assign[id]
	return s, ok
}

// Placed lists the agents that are in the office, ordered by id.
func (w *World) Placed() []Placed {
	var ids []string
	for id, b := range w.bodies {
		if b.wait <= 0 {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	talking := map[string]bool{}
	for id, a := range w.actors {
		if a.Visit != "" && w.bodies[a.Visit] != nil {
			talking[id], talking[a.Visit] = true, true
		}
	}
	out := make([]Placed, 0, len(ids))
	for _, id := range ids {
		a, b := w.actors[id], w.bodies[id]
		char := a.Character
		if char == "" {
			char = w.th.Character(a.Type)
		}
		p := Placed{ID: id, Character: char, P: b.pos, Facing: b.facing, Sub: !a.Lead}
		switch {
		case len(b.path) > 0:
			p.Anim, p.Frame = Walk, int(b.phase*walkFrames/stepsPerSec)%4
		case a.Status == Working:
			p.Anim, p.Frame = Work, int(b.phase)%4
		case a.Status == Idle:
			p.Anim, p.Frame, p.Mark = Sleep, int(b.phase/2)%2, 'z'
		}
		switch a.Status {
		case Waiting:
			p.Mark = '?'
		case Exited:
			p.Mark, p.Dim = '✓', true
		}
		if talking[id] && a.Status != Waiting {
			p.Mark = 'm'
		}
		out = append(out, p)
	}
	return out
}
