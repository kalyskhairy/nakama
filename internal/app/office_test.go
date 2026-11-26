package app

import (
	"fmt"
	"github.com/kalyskhairy/nakama/internal/live"
	"image"
	"image/color"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kalyskhairy/nakama/internal/input"
	"github.com/kalyskhairy/nakama/internal/limits"
	"github.com/kalyskhairy/nakama/internal/monitor"
	"github.com/kalyskhairy/nakama/internal/office"
	"github.com/kalyskhairy/nakama/internal/scene"
	"github.com/kalyskhairy/nakama/internal/session"
	"github.com/kalyskhairy/nakama/internal/sprites"
	"github.com/kalyskhairy/nakama/internal/theme"
	"github.com/kalyskhairy/nakama/internal/ui"
)

func solid(w, h int, c color.NRGBA) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(im.Pix); i += 4 {
		im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return im
}

// officeCfg is a small synthetic office: 400x200 background, one character, three stations.
func officeCfg(t *testing.T, cw, ch int) *OfficeConfig {
	t.Helper()
	dir := t.TempDir() + "/c"
	s := &sprites.Sheet{}
	for ri := 0; ri < 4; ri++ {
		for i := 0; i < 4; i++ {
			s.Rows[ri] = append(s.Rows[ri], solid(10, 20, color.NRGBA{200, 40, 40, 255}))
		}
	}
	for i := 0; i < 2; i++ {
		s.Rows[4] = append(s.Rows[4], solid(20, 10, color.NRGBA{200, 40, 40, 255}))
	}
	for i := 0; i < 4; i++ {
		s.Rows[5] = append(s.Rows[5], solid(12, 20, color.NRGBA{200, 40, 40, 255}))
	}
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	th := &theme.Theme{
		Background: solid(400, 200, color.NRGBA{90, 90, 90, 255}),
		CharHeight: 0.1,
		Characters: map[string]string{"c": dir},
		AgentTypes: map[string]string{"personal": "c"},
		Nodes:      map[string]theme.Point{"e": {X: 0.05, Y: 0.5}, "m": {X: 0.5, Y: 0.5}},
		Edges:      [][2]string{{"e", "m"}},
		Entrance:   "e",
		Stations: []theme.Station{
			{ID: "captain", Point: theme.Point{X: 0.8, Y: 0.5}, Kind: "work", For: []string{"session"}, Node: "m"},
			{ID: "desk1", Point: theme.Point{X: 0.6, Y: 0.4}, Kind: "work", Node: "m"},
			{ID: "desk2", Point: theme.Point{X: 0.7, Y: 0.4}, Kind: "work", Node: "m"},
			{ID: "sofa1", Point: theme.Point{X: 0.3, Y: 0.6}, Kind: "rest", Room: "r1", Node: "m"},
			{ID: "sofa2", Point: theme.Point{X: 0.2, Y: 0.6}, Kind: "rest", Room: "r2", Node: "m"},
		},
	}
	r, err := office.NewRenderer(th)
	if err != nil {
		t.Fatal(err)
	}
	return &OfficeConfig{Theme: th, Renderer: r, CellW: cw, CellH: ch}
}

const sixelStart = "\x1bP"

func officeModel(t *testing.T, info map[string]monitor.Info, subs map[string][]monitor.SubInfo, ids ...string) (*Model, *fakeMon) {
	t.Helper()
	i := 0
	m := newModel(t, func(r SpawnReq, w, h int) (*session.Session, error) {
		i++
		return spawnID(ids[i-1], "sleep", "30")(r, w, h)
	})
	fm := &fakeMon{info: info, subs: subs}
	m.SetMonitor(fm)
	m.SetOffice(officeCfg(t, 10, 20))
	m.Resize(120, 40)
	for range ids {
		m.newSession(SpawnReq{Dir: "/p"}, t0)
	}
	m.mode = input.Home
	return m, fm
}

func TestOfficeIsReachedWithV(t *testing.T) {
	m := newModel(t, spawnCmd("sleep", "30"))
	m.SetOffice(officeCfg(t, 10, 20))
	if m.Mode() != input.Work {
		t.Fatalf("the three panels are the home screen, not the office: mode %v", m.Mode())
	}
	m.HandleEvent(key("v"), t0)
	if m.Mode() != input.Home {
		t.Fatalf("v: mode %v", m.Mode())
	}
	if f := Frame(m, 120, 40, t0); !strings.Contains(f, sixelStart) {
		t.Fatal("no image in the office frame (an empty office must still be drawn)")
	}
	m.HandleEvent(key("v"), t0)
	if m.Mode() != input.Work {
		t.Fatalf("v again: mode %v", m.Mode())
	}
	m.HandleEvent(key("v"), t0)
	m.HandleEvent(key("esc"), t0)
	if m.Mode() != input.Work {
		t.Fatalf("esc: mode %v", m.Mode())
	}

	plain := newModel(t, spawnCmd("sleep", "30"))
	plain.HandleEvent(key("v"), t0)
	if plain.Mode() != input.Work || !strings.Contains(plain.Message(t0), "kantor tidak aktif") {
		t.Fatalf("no office: mode %v msg %q", plain.Mode(), plain.Message(t0))
	}

	off := newModel(t, spawnCmd("sleep", "30"))
	off.SetOffice(&OfficeConfig{Reason: "grafis dimatikan"})
	off.HandleEvent(key("v"), t0)
	if off.Mode() != input.Work || !strings.Contains(off.Message(t0), "grafis dimatikan") {
		t.Fatalf("the reason must be given: mode %v msg %q", off.Mode(), off.Message(t0))
	}
}

func TestActorsFromSessionsAndSubagents(t *testing.T) {
	subs := map[string][]monitor.SubInfo{"a": {
		{ID: "x", Type: "Explore", Description: "cari", Info: monitor.Info{Status: monitor.Working}},
		{ID: "y", Type: "Plan"},
	}}
	m, _ := officeModel(t, map[string]monitor.Info{"a": {Status: monitor.Working}}, subs, "a", "b")
	m.Tick(t0)
	as := m.Actors()
	if len(as) != 4 {
		t.Fatalf("%d actors, want 2 leads + 2 subagents: %+v", len(as), as)
	}
	var leads, subsN int
	for _, a := range as {
		if a.Lead {
			leads++
			if a.Type != "session" {
				t.Errorf("lead type %q", a.Type)
			}
		} else {
			subsN++
		}
	}
	if leads != 2 || subsN != 2 {
		t.Fatalf("leads %d subs %d", leads, subsN)
	}
}

func TestSelectionNextPrevWrapsAndIsStable(t *testing.T) {
	subs := map[string][]monitor.SubInfo{"a": {{ID: "x", Type: "Explore", Info: monitor.Info{Status: monitor.Working}}}}
	m, fm := officeModel(t, nil, subs, "a", "b")
	m.Tick(t0)
	if _, ok := m.Selected(); ok {
		t.Fatal("nothing is selected at the start")
	}
	m.HandleEvent(key("tab"), t0)
	first, _ := m.Selected()
	m.HandleEvent(key("tab"), t0)
	second, _ := m.Selected()
	if first == second {
		t.Fatal("SelectNext did not move")
	}
	m.HandleEvent(key("shift-tab"), t0)
	if got, _ := m.Selected(); got != first {
		t.Fatalf("SelectPrev: %q, want %q", got, first)
	}
	for i := 0; i < 3; i++ { // 3 actors: three steps wrap around
		m.HandleEvent(key("tab"), t0)
	}
	if got, _ := m.Selected(); got != first {
		t.Fatalf("no wrap: %q", got)
	}

	// another actor appears: the selection stays on the same one
	fm.subs["b"] = []monitor.SubInfo{{ID: "n", Type: "Plan", Info: monitor.Info{Status: monitor.Working}}}
	m.Tick(t0)
	if got, _ := m.Selected(); got != first {
		t.Fatalf("selection moved when somebody else appeared: %q", got)
	}
	// the selected actor disappears: selection moves to a neighbour, no panic
	fm.subs = nil
	for i := 0; i < 2; i++ {
		m.Tick(t0)
	}
	if _, ok := m.Selected(); !ok {
		t.Fatal("selection lost although actors remain")
	}
}

func TestOpenSelectedOpensChatOfLead(t *testing.T) {
	subs := map[string][]monitor.SubInfo{"b": {{ID: "x", Type: "Explore", Info: monitor.Info{Status: monitor.Working}}}}
	m, _ := officeModel(t, nil, subs, "a", "b")
	m.Tick(t0)
	// select the subagent of session b
	for i := 0; i < 6; i++ {
		m.HandleEvent(key("tab"), t0)
		if id, _ := m.Selected(); strings.HasPrefix(id, "sub:") {
			break
		}
	}
	if id, _ := m.Selected(); !strings.HasPrefix(id, "sub:") {
		t.Fatalf("could not select a subagent: %q", id)
	}
	m.HandleEvent(key("enter"), t0)
	if m.Mode() != input.Chat || m.FocusIndex() != 1 {
		t.Fatalf("mode %v focus %d, want chat of session b (index 1)", m.Mode(), m.FocusIndex())
	}
}

func TestOpenSelectedWithNothingSelectedDoesNothing(t *testing.T) {
	m := newModel(t, spawnCmd("sleep", "30"))
	m.SetOffice(officeCfg(t, 10, 20))
	m.mode = input.Home
	m.HandleEvent(key("enter"), t0)
	m.HandleEvent(key("tab"), t0)
	m.HandleEvent(key("shift-tab"), t0)
	if m.Mode() != input.Home || len(m.Sessions()) != 0 {
		t.Fatalf("mode %v sessions %d", m.Mode(), len(m.Sessions()))
	}
}

func TestCardRowsAppearUnderImageAndStatusLineStays(t *testing.T) {
	info := map[string]monitor.Info{"a": {Status: monitor.Working, CtxPct: 62, Model: "claude-sonnet-5-5", PendingTool: "Bash"}}
	m, _ := officeModel(t, info, nil, "a")
	m.Resize(42, 16) // the picture fills the width and leaves too few rows for a panel
	m.Tick(t0)
	m.HandleEvent(key("tab"), t0)
	m.HandleEvent(key("i"), t0)
	rows := plainRows(Frame(m, 42, 16, t0), 16)
	if !strings.Contains(rows[15], "F1 bantuan") {
		t.Fatalf("status line is not the last row: %q", rows[15])
	}
	card := strings.Join(rows[12:15], "\n")
	for _, want := range []string{"lead sesi", "sonnet-5-5", "62%", "bekerja", "alat: Bash", "Enter"} {
		if !strings.Contains(card, want) {
			t.Errorf("card lacks %q: %q", want, card)
		}
	}
	if strings.TrimSpace(strings.Join(rows[11:12], "")) != "" {
		t.Errorf("the card must sit directly above the status line, rows above are the picture: %q", rows[11:12])
	}
	m.HandleEvent(key("i"), t0)
	rows = plainRows(Frame(m, 42, 16, t0), 16)
	if strings.Contains(strings.Join(rows[12:15], ""), "Enter") {
		t.Fatal("card still shown after closing it")
	}
}

func TestFrameRelayoutsAndClearsOnResize(t *testing.T) {
	m, _ := officeModel(t, nil, nil, "a")
	if f := Frame(m, 120, 40, t0); !strings.HasPrefix(f, "\x1b[?25l\x1b[2J") {
		t.Fatalf("the first office frame must clear the screen: %q", f[:20])
	}
	if f := Frame(m, 120, 40, t0); strings.Contains(f, "\x1b[2J") {
		t.Fatal("an unchanged layout must not clear the screen again")
	}
	m.Resize(100, 30)
	if f := Frame(m, 100, 30, t0); !strings.Contains(f, "\x1b[2J") || !strings.Contains(f, sixelStart) {
		t.Fatal("after a resize the picture is redrawn from a cleared screen")
	}
	_ = Frame(m, 30, 8, t0) // far too small: text only, no panic
	m.Resize(120, 40)
	if f := Frame(m, 120, 40, t0); !strings.Contains(f, "\x1b[2J") || !strings.Contains(f, sixelStart) {
		t.Fatal("the office did not come back after being shrunk")
	}
}

func TestFrameFallsBackToTextWhenPictureDoesNotFit(t *testing.T) {
	m := newModel(t, spawnCmd("sleep", "30"))
	m.SetOffice(officeCfg(t, 1, 1)) // 1x1 pixel cells: a 45x11 screen is far too small for a 400x200 picture
	m.mode = input.Home
	f := Frame(m, 45, 12, t0)
	if strings.Contains(f, sixelStart) || !strings.Contains(ansiStrip(f), "terlalu kecil") {
		t.Fatalf("%q", ansiStrip(f))
	}
}

func TestNoImageInChatAndImageClearedWhenLeavingOffice(t *testing.T) {
	m, _ := officeModel(t, nil, nil, "a")
	Frame(m, 120, 40, t0) // office drawn
	m.openOwned(0, t0)
	f := Frame(m, 120, 40, t0)
	if strings.Contains(f, sixelStart) {
		t.Fatal("chat frame contains an image")
	}
	if !strings.Contains(f, "\x1b[2J") {
		t.Fatal("leaving the office must clear the picture off the screen")
	}
}

func TestAnimatingOnlyInOffice(t *testing.T) {
	m, _ := officeModel(t, nil, nil, "a")
	if !m.Animating() {
		t.Fatal("not animating in the office")
	}
	m.HandleEvent(key("v"), t0)
	if m.Animating() || m.Mode() != input.Work {
		t.Fatalf("panels: animating=%v mode=%v", m.Animating(), m.Mode())
	}
	m.openOwned(0, t0)
	if m.Animating() {
		t.Fatal("animating in chat")
	}
	if newModel(t, spawnCmd("sleep", "30")).Animating() {
		t.Fatal("animating without an office")
	}
}

func TestKeysWorkThroughInputOnTheOffice(t *testing.T) {
	m := newModel(t, spawnID("id-1", "sleep", "30"))
	m.SetOffice(officeCfg(t, 10, 20))
	m.Resize(120, 40)
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	m.Input([]byte("\x00"), t0) // Ctrl+Space: out of the chat
	m.Input([]byte("v"), t0)    // to the office
	if m.Mode() != input.Home {
		t.Fatalf("v: mode %v", m.Mode())
	}
	m.Tick(t0)
	m.Input([]byte("\t"), t0) // Tab selects the first actor, Enter opens its chat
	m.Input([]byte("\r"), t0)
	if m.Mode() != input.Chat || m.FocusIndex() != 0 {
		t.Fatalf("tab+enter: mode=%v focus=%d", m.Mode(), m.FocusIndex())
	}
	m.Input([]byte("\x00v"), t0)
	m.Input([]byte("\x1b[C"), t0) // right arrow
	if _, ok := m.Selected(); !ok {
		t.Fatal("arrow key selected nothing")
	}
}

func TestIdleOfficeIsNotRedrawn(t *testing.T) {
	m, _ := officeModel(t, nil, nil, "a")
	m.Tick(t0)
	if f := Frame(m, 120, 40, t0); !strings.Contains(f, sixelStart) {
		t.Fatal("the first frame must draw the picture")
	}
	if f := Frame(m, 120, 40, t0); strings.Contains(f, sixelStart) {
		t.Fatal("nothing changed but the picture was encoded and sent again")
	}
	m.Tick(t0.Add(time.Second)) // the new actor walks in: something moved
	if f := Frame(m, 120, 40, t0.Add(time.Second)); !strings.Contains(f, sixelStart) {
		t.Fatal("a moving actor must redraw")
	}
	Frame(m, 120, 40, t0.Add(time.Second))
	m.HandleEvent(key("i"), t0)
	if f := Frame(m, 120, 40, t0.Add(time.Second)); !strings.Contains(f, sixelStart) {
		t.Fatal("opening the card must redraw")
	}
	Frame(m, 120, 40, t0.Add(time.Second))
	m.HandleEvent(key("tab"), t0)
	if f := Frame(m, 120, 40, t0.Add(time.Second)); !strings.Contains(f, sixelStart) {
		t.Fatal("changing the selection must redraw")
	}
	Frame(m, 120, 40, t0.Add(time.Second))
	m.SetLimits([]limits.Usage{{Account: "personal", Err: "offline"}})
	if f := Frame(m, 120, 40, t0.Add(time.Second)); !strings.Contains(ansiStrip(f), "offline") {
		t.Fatal("a change in the status line must be drawn even when the picture is unchanged")
	}
}

var sixelRaster = regexp.MustCompile(`\x1bP[0-9;]*q"1;1;(\d+);(\d+)`)

func TestPictureNeverReachesTheStatusRows(t *testing.T) {
	for _, cell := range [][2]int{{10, 20}, {8, 16}, {16, 32}} {
		m := newModel(t, spawnCmd("sleep", "30"))
		m.SetOffice(officeCfg(t, cell[0], cell[1]))
		for _, size := range [][2]int{{120, 40}, {80, 24}, {60, 14}} {
			for _, card := range []bool{false, true} {
				m.cardOpen = card
				f := Frame(m, size[0], size[1], t0)
				if !strings.Contains(f, sixelStart) {
					continue // too small: text fallback
				}
				rm := regexp.MustCompile(`\x1b\[(\d+);(\d+)H\x1bP`).FindStringSubmatch(f)
				rs := sixelRaster.FindStringSubmatch(f)
				if rm == nil || rs == nil {
					t.Fatalf("cell %v size %v: cannot find the picture position", cell, size)
				}
				row, _ := strconv.Atoi(rm[1])
				hpx, _ := strconv.Atoi(rs[2])
				rows := (hpx + cell[1] - 1) / cell[1]
				cardRows := 0
				if card {
					cardRows = 3
				}
				if last := row + rows - 1; last > size[1]-1-cardRows-1 {
					t.Errorf("cell %v size %v card %v: picture ends on row %d, must leave a spare row above row %d", cell, size, card, last, size[1]-cardRows)
				}
			}
		}
	}
}

func TestOfficeShowsActiveSessionsFromOtherTerminals(t *testing.T) {
	m, rs, _ := workModel(t, nil, ent("busy", "/p", "Di terminal lain", time.Hour), ent("old", "/p", "Lama", 3*time.Hour))
	m.SetLive([]live.Proc{{PID: 99999, SessionID: "busy"}})
	m.SetOffice(officeCfg(t, 10, 20))
	m.HandleEvent(key("v"), t0)
	m.Tick(t0)
	if as := m.Actors(); len(as) != 1 || as[0].ID != "busy" {
		t.Fatalf("the office must show the active session, not only nakama's own: %+v", as)
	}
	m.HandleEvent(key("tab"), t0)
	m.HandleEvent(key("enter"), t0)
	if len(rs.reqs) != 0 || m.Mode() != input.Home || !strings.Contains(m.Message(t0), "terminal lain") {
		t.Fatalf("a session of another terminal must not be resumed: spawns %d mode %v msg %q", len(rs.reqs), m.Mode(), m.Message(t0))
	}
}

func pictureSize(t *testing.T, frame string) (w, h int) {
	t.Helper()
	rs := sixelRaster.FindStringSubmatch(frame)
	if rs == nil {
		t.Fatal("no picture in the frame")
	}
	w, _ = strconv.Atoi(rs[1])
	h, _ = strconv.Atoi(rs[2])
	return
}

func TestOfficeTurnsUprightInATallTerminal(t *testing.T) {
	m, _ := officeModel(t, nil, nil, "a")
	m.Resize(40, 50) // 400x1000 px: upright gives 200x400, lying 400x200 - equal, so make it taller
	m.SetCellSize(8, 20)
	if w, h := pictureSize(t, Frame(m, 40, 50, t0)); h <= w {
		t.Fatalf("tall terminal: picture %dx%d must be upright", w, h)
	}
	m.Resize(160, 30)
	m.SetCellSize(10, 20)
	if w, h := pictureSize(t, Frame(m, 160, 30, t0)); w <= h {
		t.Fatalf("wide terminal: picture %dx%d must lie flat", w, h)
	}
}

func TestOfficeCrewPanelUsesTheSpareSpace(t *testing.T) {
	info := map[string]monitor.Info{"a": {Status: monitor.Working, PendingTool: "Bash", CtxPct: 7}}
	m, _ := officeModel(t, info, nil, "a")
	m.Tick(t0)
	f := ansiStrip(Frame(m, 120, 40, t0))
	if !strings.Contains(f, "Kru (1)") || !strings.Contains(f, "alat: Bash") {
		t.Fatalf("spare columns must list the crew: %q", f[max(0, len(f)-600):])
	}
	m.Resize(42, 13) // the picture fills it: no room for a panel
	m.SetCellSize(10, 20)
	if f := ansiStrip(Frame(m, 42, 13, t0)); strings.Contains(f, "Kru (") {
		t.Fatal("a panel without room for it")
	}
}

func TestCellSizeReportIsNotTypedIntoClaude(t *testing.T) {
	spawn, out := recordAll(t, "")
	m := newModel(t, spawn)
	m.SetOffice(officeCfg(t, 8, 16))
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	time.Sleep(300 * time.Millisecond)
	m.Input([]byte("\x1b[6;20;10tok"), t0)
	waitFor(t, "only the typed text", fileHas(out, "ok"))
	if m.office.CellW != 10 || m.office.CellH != 20 {
		t.Fatalf("cell size %dx%d", m.office.CellW, m.office.CellH)
	}
}

func TestBusyRecordMeansWorking(t *testing.T) {
	m, _, fm := workModel(t, nil, ent("x", "/p", "Di terminal lain", time.Hour))
	m.SetOffice(officeCfg(t, 10, 20))
	m.SetLive([]live.Proc{{PID: 99999, SessionID: "x", Status: "busy"}})
	m.Tick(t0)
	if r := m.activeRows(t0); len(r) != 1 || r[0].State != ui.StateWorking {
		t.Fatalf("Claude says busy: %+v", r)
	}
	if a := m.Actors(); len(a) != 1 || a[0].Status != scene.Working {
		t.Fatalf("the character must work: %+v", a)
	}
	fm.info = map[string]monitor.Info{"x": {Status: monitor.Waiting}}
	if r := m.activeRows(t0); r[0].State != ui.StateWaiting {
		t.Fatalf("waiting for the owner must win over busy: %+v", r)
	}
	fm.info = nil
	m.SetLive([]live.Proc{{PID: 99999, SessionID: "x", Status: "idle"}})
	if r := m.activeRows(t0); r[0].State != ui.StateIdle {
		t.Fatalf("an idle record changes nothing: %+v", r)
	}
}

func TestExitedSessionLeavesTheOffice(t *testing.T) {
	m, _ := officeModel(t, nil, nil, "a", "b")
	m.Tick(t0)
	m.Sessions()[0].Close()
	waitFor(t, "exit", m.Sessions()[0].Exited)
	m.Tick(t0.Add(time.Second))
	if as := m.Actors(); len(as) != 1 || as[0].ID != "b" {
		t.Fatalf("a finished session must leave the ship: %+v", as)
	}
	if got := strings.Join(m.crewLines(40, 20, t0), "\n"); !strings.Contains(got, "Kru (1)") {
		t.Fatalf("and the crew list:\n%s", got)
	}
	if n := len(m.activeRows(t0)); n != 2 {
		t.Fatalf("Aktif keeps it, so x can close it: %d", n)
	}
}

func TestOnlyTheChangedPartsOfThePictureAreSent(t *testing.T) {
	m, _ := officeModel(t, map[string]monitor.Info{"a": {Status: monitor.Working}}, nil, "a")
	m.SetCellSize(10, 20)
	m.office.Theme.AgentTypes["session"] = "c" // or the character is not drawn at all
	m.Tick(t0)
	first := Frame(m, 120, 40, t0)
	at := t0
	step := func() string {
		at = at.Add(400 * time.Millisecond)
		m.Tick(at)
		return Frame(m, 120, 40, at)
	}
	step()
	second := step() // the character takes another step
	if !strings.Contains(first, sixelStart) || !strings.Contains(second, sixelStart) {
		t.Fatal("both frames must carry a picture")
	}
	area := func(f string) int { // pixels painted, from each picture's raster attributes
		n := 0
		for _, s := range strings.Split(f, sixelStart)[1:] {
			var w, h int
			fmt.Sscanf(s[strings.Index(s, "\"1;1;"):], "\"1;1;%d;%d", &w, &h)
			n += w * h
		}
		return n
	}
	if strings.Contains(second, "\x1b[2J") || area(second) == 0 || area(second)*2 > area(first) {
		t.Fatalf("a step repaints %d of %d pixels", area(second), area(first))
	}
}
