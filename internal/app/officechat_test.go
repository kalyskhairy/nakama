package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/kalyskhairy/nakama/internal/input"
	"github.com/kalyskhairy/nakama/internal/monitor"
)

func inOfficeWithSession(t *testing.T, spawn Spawn) *Model {
	t.Helper()
	m := newModel(t, spawn)
	m.SetOffice(officeCfg(t, 10, 20))
	m.Resize(120, 40)
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	m.HandleEvent(key("toggle"), t0)
	m.HandleEvent(key("v"), t0)
	m.Tick(t0)
	m.HandleEvent(key("tab"), t0)
	return m
}

func TestOfficeChatPanelTypesIntoTheSelectedSession(t *testing.T) {
	spawn, out := recordAll(t, "")
	m := inOfficeWithSession(t, spawn)
	m.HandleEvent(key("enter"), t0)
	if m.Mode() != input.Chat || !m.inOffice {
		t.Fatalf("Enter: mode %v inOffice %v", m.Mode(), m.inOffice)
	}
	time.Sleep(300 * time.Millisecond)
	m.Input([]byte("halo"), t0)
	waitFor(t, "typed text at the child", fileHas(out, "halo"))
	f := Frame(m, 120, 40, t0)
	if !strings.Contains(f, sixelStart) || !strings.Contains(ansiStrip(f), "Chat ──") {
		t.Fatal("the ship and the chat panel must be on one screen")
	}
	m.Input([]byte("\x00"), t0)
	if m.Mode() != input.Home {
		t.Fatalf("Ctrl+Space must go back to picking crew: %v", m.Mode())
	}
	m.Input([]byte("v"), t0)
	if m.Mode() != input.Work || m.inOffice {
		t.Fatalf("v: mode %v inOffice %v", m.Mode(), m.inOffice)
	}
}

func TestOfficeChatSizesThePTYToThePanel(t *testing.T) {
	script := `trap 'stty size' WINCH; stty size; while :; do sleep 0.1; done`
	m := inOfficeWithSession(t, spawnID("s", "sh", "-c", script))
	m.HandleEvent(key("enter"), t0)
	l, ok := m.officeLayout(120, 40)
	if !ok || l.chat.W != 120 || l.chat.H < 12 {
		t.Fatalf("tall layout: %+v %v", l, ok)
	}
	want := fmt.Sprintf("%d %d", l.chat.H, l.chat.W)
	waitFor(t, "PTY size "+want, func() bool { return strings.HasSuffix(strings.TrimSpace(m.Sessions()[0].Text()), want) })
}

func TestOfficeLayoutWideHasTwoColumns(t *testing.T) {
	m := newModel(t, spawnCmd("sleep", "30"))
	l, ok := m.officeLayout(180, 45)
	if !ok || l.chat.W < 80 || l.head.X != l.chat.X || l.chat.H != 43 {
		t.Fatalf("chat is the whole right column: %+v %v", l, ok)
	}
	// left column: the ship on top, the crew below it
	if l.crew.X != 0 || l.crew.W != l.shipCols || l.shipCols >= l.chat.X || l.crew.Y != l.shipRows+1 || l.crew.Y+l.crew.H != 44 || l.crew.H < 14 {
		t.Fatalf("%+v", l)
	}
	if _, ok := m.officeLayout(180, 20); ok {
		t.Fatal("too low for a chat panel")
	}
}

func TestWideCrewPanelPutsTheCardBesideTheList(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e", "f"}
	m, _ := officeModel(t, nil, nil, ids...)
	m.Tick(t0)
	m.selected = "f"
	lines := m.crewLines(90, 16, t0)
	all := strings.Join(lines, "\n")
	if len(lines) != 16 || !strings.Contains(lines[0], "Crew (6)") || !strings.Contains(lines[0], "│") {
		t.Fatalf("two columns:\n%s", all)
	}
	if !strings.Contains(all, "SELECTED AGENT") || !strings.Contains(all, "\n>") {
		t.Fatalf("the selected one stays visible in a short list:\n%s", all)
	}
	for _, l := range lines {
		if w := ansi.StringWidth(l); w != 90 {
			t.Fatalf("width %d: %q", w, l)
		}
	}
}

func TestTypingInTheOfficeDoesNotResendThePicture(t *testing.T) {
	m := inOfficeWithSession(t, spawnID("s", "sh", "-c", "sleep 0.5; echo muncul-belakangan; sleep 30"))
	m.HandleEvent(key("enter"), t0)
	if f := Frame(m, 120, 40, t0); !strings.Contains(f, sixelStart) {
		t.Fatal("first frame")
	}
	waitFor(t, "output", func() bool { return strings.Contains(m.Sessions()[0].Text(), "muncul-belakangan") })
	f := Frame(m, 120, 40, t0)
	if strings.Contains(f, sixelStart) || strings.Contains(f, "\x1b[2J") {
		t.Fatal("new chat output must repaint the text only")
	}
	if !strings.Contains(ansiStrip(f), "muncul-belakangan") {
		t.Fatalf("chat output missing: %q", ansiStrip(f))
	}
}

func TestOfficeEnterResumesInThePanel(t *testing.T) {
	m, rs, _ := workModel(t, nil, ent("old", "/p", "J", time.Minute))
	m.SetOffice(officeCfg(t, 10, 20))
	m.Resize(120, 40)
	m.HandleEvent(key("v"), t0)
	m.Tick(t0)
	m.HandleEvent(key("tab"), t0)
	m.HandleEvent(key("enter"), t0)
	if len(rs.reqs) != 1 || rs.reqs[0].ResumeID != "old" || m.Mode() != input.Chat || !m.inOffice {
		t.Fatalf("%+v mode %v inOffice %v", rs.reqs, m.Mode(), m.inOffice)
	}
}

func TestOfficeZoomAndSmallTerminalOpenTheFullChat(t *testing.T) {
	m := inOfficeWithSession(t, spawnCmd("sleep", "30"))
	m.HandleEvent(key("z"), t0)
	if m.Mode() != input.Chat || m.inOffice || !m.zoom {
		t.Fatalf("z: mode %v inOffice %v zoom %v", m.Mode(), m.inOffice, m.zoom)
	}
	m = inOfficeWithSession(t, spawnCmd("sleep", "30"))
	m.Resize(42, 16)
	m.HandleEvent(key("enter"), t0)
	if m.Mode() != input.Chat || m.inOffice {
		t.Fatalf("no room for a panel: mode %v inOffice %v", m.Mode(), m.inOffice)
	}
}

func TestCrewPanelListsSubagentsUnderTheirLead(t *testing.T) {
	subs := map[string][]monitor.SubInfo{"a": {{ID: "x", Type: "Explore", Description: "cari berkas", Info: monitor.Info{Status: monitor.Working}}}}
	m, _ := officeModel(t, nil, subs, "a")
	m.Tick(t0)
	got := strings.Join(m.crewLines(40, 30, t0), "\n")
	if !strings.Contains(got, "└● Explore · cari berkas") {
		t.Fatalf("subagent missing from the crew:\n%s", got)
	}
	m.selected = "sub:a:x"
	got = strings.Join(m.crewLines(40, 30, t0), "\n")
	if !strings.Contains(got, ">  └● Explore") || strings.Contains(got, "\n>● ") {
		t.Fatalf("only the subagent is marked as selected:\n%s", got)
	}
	for i := 1; i <= 5; i++ { // newcomers walk in one second apart
		m.Tick(t0.Add(time.Duration(i) * time.Second))
	}
	var subSeen, leadSeen bool
	for _, p := range m.world.Placed() {
		subSeen = subSeen || (p.Sub && p.ID == "sub:a:x")
		leadSeen = leadSeen || (!p.Sub && p.ID == "a")
	}
	if !subSeen || !leadSeen {
		t.Fatalf("placed: %+v", m.world.Placed())
	}
}

func TestSelectedCardInTheCrewPanelHasOneFactPerLine(t *testing.T) {
	info := map[string]monitor.Info{"a": {Status: monitor.Working, CtxPct: 62, Model: "claude-opus-5-5", PendingTool: "Bash"}}
	m, _ := officeModel(t, info, nil, "a")
	m.Tick(t0)
	m.selected = "a"
	long := m.cards["a"]
	long.title = "Fix the shipping cost calculation in the shopping cart"
	m.cards["a"] = long
	lines := m.crewLines(30, 30, t0)
	var got []string
	for _, l := range lines {
		got = append(got, strings.TrimSpace(l))
	}
	all := "\n" + strings.Join(got, "\n") + "\n"
	for _, want := range []string{"SELECTED AGENT", "session lead", "model: opus-5-5", "status: working", "context: 62%", "tool: Bash", "Enter = talk in chat"} {
		if !strings.Contains(all, "\n"+want+"\n") {
			t.Errorf("no line %q in:%s", want, all)
		}
	}
	if !strings.Contains(strings.Join(strings.Fields(all), " "), "Fix the shipping cost calculation in the shopping cart") {
		t.Errorf("the title must be whole:%s", all)
	}
}

func TestPictureWaitsWhileTheOwnerTypes(t *testing.T) {
	m := inOfficeWithSession(t, spawnCmd("sleep", "30"))
	m.HandleEvent(key("enter"), t0)
	Frame(m, 120, 40, t0)
	picture := func(at time.Duration, who string) bool {
		m.selected = who // anything that changes the picture
		return strings.Contains(Frame(m, 120, 40, t0.Add(at)), sixelStart)
	}
	m.Input([]byte("a"), t0.Add(time.Second))
	if picture(1200*time.Millisecond, "x1") {
		t.Fatal("200 ms after a keystroke the picture must wait")
	}
	if !picture(2500*time.Millisecond, "x2") {
		t.Fatal("after a pause in typing the picture must catch up")
	}
	if picture(2700*time.Millisecond, "x3") {
		t.Fatal("with the focus in the chat: at most 2 pictures a second")
	}
	if !picture(3100*time.Millisecond, "x4") {
		t.Fatal("half a second later it may draw again")
	}
}
