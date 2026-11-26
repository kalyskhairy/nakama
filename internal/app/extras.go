package app

import (
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/kalyskhairy/nakama/internal/input"
	"github.com/kalyskhairy/nakama/internal/monitor"
)

// NoticeKind names what a notice is about, so the owner can choose which ones to get.
type NoticeKind string

const (
	NoticeWaiting  NoticeKind = "waiting"
	NoticeFinished NoticeKind = "finished"
	NoticeContext  NoticeKind = "context"
)

// Notice is something the owner should hear about even when looking elsewhere.
type Notice struct {
	Title, Text string
	Kind        NoticeKind
}

type watch struct {
	waiting, working bool
	full             bool      // warned that its context is filling up
	quietSince       time.Time // when a working session stopped
}

const (
	finishedAfter = 3 * time.Second // shorter pauses are just the gap between two tools
	attended      = 30 * time.Second
	visitFor      = 25 * time.Second
	ctxWarn       = 80 // per cent of the context window
	ctxCalm       = 70 // below this a new warning is due next time
)

// TakeNotices hands over the notices gathered since the last call.
func (m *Model) TakeNotices() []Notice {
	out := m.notices
	m.notices = nil
	return out
}

// watchCrew notes which sessions started waiting for the owner or finished their work.
func (m *Model) watchCrew(now time.Time) {
	next := map[string]*watch{}
	for _, r := range m.crewRows(now) {
		in := m.info(r.ID, now)
		st := in.Status
		w, known := m.seen[r.ID]
		if !known {
			next[r.ID] = &watch{waiting: st == monitor.Waiting, working: st == monitor.Working, full: in.CtxPct >= ctxWarn}
			continue
		}
		next[r.ID] = w
		say, kind := "", NoticeFinished
		switch st {
		case monitor.Waiting:
			if !w.waiting {
				say, kind = "is waiting for you", NoticeWaiting
			}
			w.quietSince = time.Time{}
		case monitor.Working:
			w.working, w.quietSince = true, time.Time{}
		default:
			if w.working && w.quietSince.IsZero() {
				w.quietSince = now
			}
			if w.working && now.Sub(w.quietSince) >= finishedAfter {
				w.working, say = false, "finished working"
			}
		}
		w.waiting = st == monitor.Waiting
		who := r.Folder
		if name := charName(m.cast[r.ID]); name != "" {
			who = name + " · " + who
		}
		switch {
		case in.CtxPct >= ctxWarn && !w.full:
			w.full = true
			m.notices = append(m.notices, Notice{Title: fmt.Sprintf("%s: context %d%% full", who, in.CtxPct), Text: r.Title + " — consider /compact or a new session", Kind: NoticeContext})
		case in.CtxPct >= 0 && in.CtxPct < ctxCalm:
			w.full = false
		}
		beingUsed := m.mode == input.Chat && m.selectedID(now) == r.ID && now.Sub(m.lastTyped) < attended
		if say != "" && !beingUsed {
			m.notices = append(m.notices, Notice{Title: who + " " + say, Text: r.Title + " — " + say, Kind: kind})
		}
	}
	m.seen = next
}

func charName(c string) string {
	if c == "" {
		return ""
	}
	return strings.ToUpper(c[:1]) + c[1:]
}

// visiting is the session this one just sent a message to, if that is one of the crew.
func (m *Model) visiting(id string, in monitor.Info, now time.Time) string {
	if in.SentTo == "" || now.Sub(in.SentAt) > visitFor {
		return ""
	}
	name, _, _ := strings.Cut(in.SentTo, " [") // "name [ref]"
	for _, p := range m.procs {
		if p.Name == name && p.SessionID != id {
			return p.SessionID
		}
	}
	return ""
}

// agentAt is the agent a click on the office screen points at: a character on the ship, or a line
// of the crew list.
func (m *Model) agentAt(col, row int) string {
	if id := m.actorAt(col, row); id != "" {
		return id
	}
	if m.officeShown() && m.drewOffice && m.crewArea.Contains(col, row) {
		if i := row - m.crewArea.Y - 1; i >= 0 && i < len(m.crewIDs) {
			if _, ok := m.owner[m.crewIDs[i]]; ok {
				return m.crewIDs[i]
			}
		}
	}
	return ""
}

// actorAt is the actor drawn at a screen cell of the office, the one in front if several are.
func (m *Model) actorAt(col, row int) string {
	if !m.officeShown() || !m.drewOffice || m.world == nil {
		return ""
	}
	l, cw, ch := m.shipLay, m.office.CellW, m.office.CellH
	x, y := (col-l.Col+1)*cw, (row-l.Row+1)*ch
	if x < 0 || y < 0 || x >= l.ImgW || y >= l.ImgH {
		return ""
	}
	hit, front := "", -1.0
	for _, p := range m.world.Placed() {
		b := m.views[m.view].r.Bounds(p, l.ImgW, l.ImgH, l.Scale)
		if !p.Idle && b.Overlaps(imageRect(x, y, cw, ch)) && p.P.Y > front {
			hit, front = p.ID, p.P.Y
		}
	}
	return hit
}

// imageRect is the picture area of the clicked cell.
func imageRect(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }
