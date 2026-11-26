package app

import (
	"strings"
	"testing"
	"time"

	"github.com/kalyskhairy/nakama/internal/input"
	"github.com/kalyskhairy/nakama/internal/live"
	"github.com/kalyskhairy/nakama/internal/monitor"
)

func TestNoticeWhenASessionWaitsOrFinishes(t *testing.T) {
	m, fm := officeModel(t, map[string]monitor.Info{"a": {Status: monitor.Working}}, nil, "a")
	at := func(sec int) []Notice { m.Poll(t0.Add(time.Duration(sec) * time.Second)); return m.TakeNotices() }
	if got := at(0); len(got) != 0 {
		t.Fatalf("the first look is no news: %v", got)
	}
	fm.info["a"] = monitor.Info{Status: monitor.Waiting}
	if got := at(1); len(got) != 1 || !strings.Contains(got[0].Text, "menunggu") {
		t.Fatalf("waiting: %v", got)
	}
	if got := at(2); len(got) != 0 {
		t.Fatalf("only once: %v", got)
	}
	fm.info["a"] = monitor.Info{Status: monitor.Working}
	at(3)
	fm.info["a"] = monitor.Info{Status: monitor.Idle}
	if got := at(4); len(got) != 0 {
		t.Fatalf("a pause between tools is not the end: %v", got)
	}
	if got := at(8); len(got) != 1 || !strings.Contains(got[0].Text, "selesai") {
		t.Fatalf("finished: %v", got)
	}
}

func TestCrewListNamesTheCharacter(t *testing.T) {
	m, _ := officeModel(t, nil, nil, "a")
	m.office.Theme.SessionCharacters = []string{"c"}
	m.Tick(t0)
	m.selected = "a"
	got := strings.Join(m.crewLines(80, 20, t0), "\n")
	if !strings.Contains(got, "C · [") || !strings.Contains(got, "lead sesi · C") {
		t.Fatalf("no character name:\n%s", got)
	}
}

func TestClickOnACharacterOpensItsChat(t *testing.T) {
	m, _ := officeModel(t, nil, nil, "a")
	m.office.Theme.AgentTypes["session"] = "c"
	m.SetCellSize(10, 20)
	m.Resize(200, 45)
	m.mode = input.Home
	for i := 0; i < 30; i++ {
		m.Tick(t0.Add(time.Duration(i) * 400 * time.Millisecond))
	}
	Frame(m, 200, 45, t0)
	p := m.world.Placed()[0]
	x := m.shipLay.Col - 1 + int(p.P.X*float64(m.shipLay.ImgW))/10
	y := m.shipLay.Row - 1 + (int(p.P.Y*float64(m.shipLay.ImgH))-5)/20
	m.HandleEvent(input.Event{Kind: input.EvMouse, Press: true, X: 0, Y: 0}, t0)
	if m.Mode() != input.Home {
		t.Fatal("a click on the sea does nothing")
	}
	m.HandleEvent(input.Event{Kind: input.EvMouse, Press: true, X: x, Y: y}, t0)
	if m.selected != "a" || m.Mode() != input.Chat || !m.inOffice {
		t.Fatalf("selected %q mode %v", m.selected, m.Mode())
	}
}

func TestAMessageSendsTheSenderToTheReceiver(t *testing.T) {
	m, fm := officeModel(t, map[string]monitor.Info{}, nil, "a", "b")
	m.SetLive([]live.Proc{{SessionID: "b", Name: "peer-b"}})
	fm.info["a"] = monitor.Info{SentTo: "peer-b [1a2b]", SentAt: t0}
	visit := func(at time.Duration) string {
		m.Tick(t0.Add(at))
		for _, a := range m.Actors() {
			if a.ID == "a" {
				return a.Visit
			}
		}
		return "?"
	}
	if got := visit(time.Second); got != "b" {
		t.Fatalf("visit %q", got)
	}
	if got := strings.Join(m.crewLines(80, 20, t0.Add(time.Second)), "\n"); !strings.Contains(got, "→ pesan ke") {
		t.Fatalf("the list says nothing:\n%s", got)
	}
	if got := visit(time.Minute); got != "" {
		t.Fatalf("an old message: still visiting %q", got)
	}
	fm.info["a"] = monitor.Info{SentTo: "agent-xyz", SentAt: t0}
	if got := visit(time.Second); got != "" {
		t.Fatalf("not a session: %q", got)
	}
}
