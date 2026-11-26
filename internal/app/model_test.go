package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kalyskhairy/nakama/internal/index"
	"github.com/kalyskhairy/nakama/internal/input"
	"github.com/kalyskhairy/nakama/internal/limits"
	"github.com/kalyskhairy/nakama/internal/live"
	"github.com/kalyskhairy/nakama/internal/monitor"
	"github.com/kalyskhairy/nakama/internal/prefs"
	"github.com/kalyskhairy/nakama/internal/session"
	"github.com/kalyskhairy/nakama/internal/workspace"
)

var t0 = time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)

func spawnCmd(name string, args ...string) Spawn {
	return func(r SpawnReq, w, h int) (*session.Session, error) {
		return session.Start(session.Config{Cmd: name, Args: args, W: w, H: h}, nil)
	}
}

func spawnID(id, name string, args ...string) Spawn {
	return func(r SpawnReq, w, h int) (*session.Session, error) {
		return session.Start(session.Config{Cmd: name, Args: args, W: w, H: h, ID: id}, nil)
	}
}

func shownPrefs(t *testing.T, cwds ...string) *prefs.Folders {
	t.Helper()
	f := prefs.Load(filepath.Join(t.TempDir(), "folders.json"))
	for _, c := range cwds {
		f.Set(c, true)
	}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	return f
}

func newModel(t *testing.T, spawn Spawn) *Model {
	t.Helper()
	m := NewModel(spawn, 140, 40)
	m.SetPrefs(shownPrefs(t))
	m.statDir = func(string) bool { return true }
	t.Cleanup(func() {
		for _, s := range m.Sessions() {
			s.Close()
		}
	})
	return m
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func key(k string) input.Event { return input.Event{Kind: input.EvKey, Key: k, Raw: []byte(k)} }

func click(x, y int) input.Event { return input.Event{Kind: input.EvMouse, X: x, Y: y, Press: true} }

func ent(id, cwd, title string, age time.Duration) index.Entry {
	return index.Entry{ID: id, Cwd: cwd, Title: title, Account: "personal", ConfigDir: "/cfg/.claude", ModTime: t0.Add(-age)}
}

type fakeMon struct {
	info   map[string]monitor.Info
	subs   map[string][]monitor.SubInfo
	polled []string
}

func (f *fakeMon) Subagents(id string, now time.Time) []monitor.SubInfo { return f.subs[id] }
func (f *fakeMon) Activity(id string) []monitor.Activity                { return nil }
func (f *fakeMon) Poll(id string, now time.Time)                        { f.polled = append(f.polled, id) }
func (f *fakeMon) Info(id string, now time.Time) monitor.Info {
	if i, ok := f.info[id]; ok {
		return i
	}
	return monitor.Info{CtxPct: -1}
}

// recSpawn starts `sleep 30` for every request and records the requests.
type recSpawn struct{ reqs []SpawnReq }

func (r *recSpawn) spawn(req SpawnReq, w, h int) (*session.Session, error) {
	r.reqs = append(r.reqs, req)
	return session.Start(session.Config{Cmd: "sleep", Args: []string{"30"}, W: w, H: h, ID: req.ResumeID}, nil)
}

// workModel is a model over the given sessions on disk, with a fake monitor and a recording spawner.
func workModel(t *testing.T, shown []string, es ...index.Entry) (*Model, *recSpawn, *fakeMon) {
	t.Helper()
	rs, fm := &recSpawn{}, &fakeMon{}
	m := newModel(t, rs.spawn)
	m.SetPrefs(shownPrefs(t, shown...))
	m.SetMonitor(fm)
	m.SetIndex(es)
	return m, rs, fm
}

func selectSession(m *Model, id string) {
	m.pane, m.treeKey = workspace.PaneTree, "s:"+id
}

func TestEnterResumesOldSessionWithConfigDir(t *testing.T) {
	e := ent("old", "/proj", "Judul", time.Hour)
	e.Account, e.ConfigDir = "work", "/home/k/.claude-work"
	m, rs, _ := workModel(t, []string{"/proj"}, e)
	selectSession(m, "old")
	m.HandleEvent(key("enter"), t0)
	if len(rs.reqs) != 1 || rs.reqs[0] != (SpawnReq{Dir: "/proj", ConfigDir: "/home/k/.claude-work", ResumeID: "old"}) {
		t.Fatalf("%+v", rs.reqs)
	}
	if m.Mode() != input.Chat {
		t.Fatalf("mode %v", m.Mode())
	}
}

func TestEnterOnOwnedSwitchesWithoutSpawn(t *testing.T) {
	m, rs, _ := workModel(t, []string{"/proj"}, ent("old", "/proj", "J", time.Hour))
	selectSession(m, "old")
	m.HandleEvent(key("enter"), t0)
	m.HandleEvent(key("toggle"), t0)
	if m.Mode() != input.Work {
		t.Fatalf("toggle left mode %v", m.Mode())
	}
	m.HandleEvent(key("enter"), t0)
	if len(rs.reqs) != 1 || m.Mode() != input.Chat {
		t.Fatalf("spawns %d mode %v", len(rs.reqs), m.Mode())
	}
}

func TestEnterOnClaimedRefuses(t *testing.T) {
	m, rs, _ := workModel(t, []string{"/proj"}, ent("busy", "/proj", "J", time.Hour))
	m.SetLive([]live.Proc{{PID: 99999, SessionID: "busy"}})
	selectSession(m, "busy")
	m.HandleEvent(key("enter"), t0)
	if len(rs.reqs) != 0 || m.Mode() != input.Work || !strings.Contains(m.Message(t0), "terminal lain") {
		t.Fatalf("spawns %d mode %v msg %q", len(rs.reqs), m.Mode(), m.Message(t0))
	}
}

func TestEnterOnRecentNeedsSecondEnter(t *testing.T) {
	m, rs, _ := workModel(t, []string{"/proj"}, ent("hot", "/proj", "J", 10*time.Second))
	selectSession(m, "hot")
	m.HandleEvent(key("enter"), t0)
	if len(rs.reqs) != 0 || !strings.Contains(m.Message(t0), "Enter lagi") {
		t.Fatalf("first enter: spawns %d msg %q", len(rs.reqs), m.Message(t0))
	}
	m.HandleEvent(key("enter"), t0.Add(6*time.Second)) // the confirmation expired
	if len(rs.reqs) != 0 {
		t.Fatal("an expired confirmation must ask again")
	}
	m.HandleEvent(key("enter"), t0.Add(8*time.Second))
	if len(rs.reqs) != 1 {
		t.Fatalf("second enter did not resume: %d", len(rs.reqs))
	}
}

func TestEnterWithMissingCwdSaysSoAndDoesNotSpawn(t *testing.T) {
	for _, cwd := range []string{`D:\webshop`, "/home/k/deleted", ""} {
		m, rs, _ := workModel(t, []string{cwd}, ent("old", cwd, "J", time.Hour))
		m.statDir = isDir // the real check
		selectSession(m, "old")
		m.HandleEvent(key("enter"), t0)
		if len(rs.reqs) != 0 || m.Mode() != input.Work || m.Message(t0) == "" {
			t.Errorf("cwd %q: spawns %d mode %v msg %q", cwd, len(rs.reqs), m.Mode(), m.Message(t0))
		}
	}
}

func TestOwnedSessionFollowsIDFromLiveRecord(t *testing.T) {
	m := newModel(t, spawnID("A", "sleep", "30"))
	fm := &fakeMon{}
	m.SetMonitor(fm)
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	m.SetLive([]live.Proc{{PID: m.Sessions()[0].PID(), SessionID: "B"}})
	m.Poll(t0)
	got := strings.Join(fm.polled, ",")
	if !strings.Contains(got, "B") || strings.Contains(got, "A") {
		t.Fatalf("polled %q: the id Claude records under the process id must win", got)
	}
}

func TestWaitingInHiddenFolderCountsInSummary(t *testing.T) {
	m, _, fm := workModel(t, []string{"/other"}, ent("w", "/hidden", "T", 10*time.Second))
	fm.info = map[string]monitor.Info{"w": {Status: monitor.Waiting}}
	if got := m.Summary(t0).Waiting; got != 1 {
		t.Fatalf("Waiting = %d", got)
	}
	if rows := m.treeRows(t0); len(rows) != 0 {
		t.Fatalf("hidden folder shown in the tree: %+v", rows)
	}
	if rows := m.activeRows(t0); len(rows) != 1 {
		t.Fatalf("the waiting session must stay in Aktif: %+v", rows)
	}
}

func TestSubagentWaitingCountsAndExitedSessionDoesNot(t *testing.T) {
	m := newModel(t, spawnID("a", "sleep", "30"))
	fm := &fakeMon{info: map[string]monitor.Info{"a": {Status: monitor.Working}},
		subs: map[string][]monitor.SubInfo{"a": {{ID: "x", Type: "Explore", Info: monitor.Info{Status: monitor.Waiting}}}}}
	m.SetMonitor(fm)
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	if got := m.Summary(t0).Waiting; got != 1 {
		t.Fatalf("Waiting = %d, want the subagent", got)
	}
	if rows := m.activeRows(t0); len(rows) != 1 || rows[0].State != 3 { // ui.StateWaiting
		t.Fatalf("the lead of a waiting subagent must show as waiting: %+v", rows)
	}
	m.Sessions()[0].Close()
	waitFor(t, "exit", m.Sessions()[0].Exited)
	if got := m.Summary(t0).Waiting; got != 0 {
		t.Fatalf("an exited session still counts as waiting: %d", got)
	}
}

func TestSummaryTokensSumAcrossActiveSessions(t *testing.T) {
	m, _, fm := workModel(t, []string{"/p"}, ent("a", "/p", "", time.Second), ent("b", "/p", "", 2*time.Second), ent("old", "/p", "", 3*time.Hour))
	fm.info = map[string]monitor.Info{"a": {TokensToday: 100}, "b": {TokensToday: 250}, "old": {TokensToday: 9999}}
	if got := m.Summary(t0).TokensToday; got != 350 {
		t.Fatalf("TokensToday = %d (sessions outside Aktif must not count)", got)
	}
}

func TestPollOnlyActiveAndSelected(t *testing.T) {
	m, _, fm := workModel(t, []string{"/p"}, ent("a", "/p", "", time.Second), ent("old", "/p", "", 3*time.Hour), ent("old2", "/p", "", 4*time.Hour))
	m.Poll(t0)
	if len(fm.polled) != 1 || fm.polled[0] != "a" {
		t.Fatalf("polled %v", fm.polled)
	}
	fm.polled = nil
	selectSession(m, "old2")
	m.Poll(t0)
	if len(fm.polled) != 2 {
		t.Fatalf("the selected session must be polled too: %v", fm.polled)
	}
}

func TestMouseToggleKey(t *testing.T) {
	m := newModel(t, spawnCmd("sleep", "30"))
	if !m.MouseOn() {
		t.Fatal("mouse must be on by default")
	}
	m.HandleEvent(key("m"), t0)
	if m.MouseOn() {
		t.Fatal("m did not turn the mouse off")
	}
	m.HandleEvent(key("m"), t0)
	if !m.MouseOn() {
		t.Fatal("m did not turn the mouse on")
	}
}

func TestMouseClickSelectsRowAndDoubleClickOpens(t *testing.T) {
	m, rs, _ := workModel(t, []string{"/p"}, ent("a", "/p", "A", time.Hour), ent("b", "/p", "B", 2*time.Hour))
	r := workspace.Compute(m.w, m.h, len(m.activeRows(t0)), false, false)
	// tree rows: folder /p, session a, session b
	m.HandleEvent(click(r.Tree.X+3, r.Tree.Y+1+2), t0)
	if m.treeKey != "s:b" || m.pane != workspace.PaneTree {
		t.Fatalf("click did not select: pane %v key %q", m.pane, m.treeKey)
	}
	m.HandleEvent(click(r.Tree.X+3, r.Tree.Y+1+2), t0.Add(100*time.Millisecond))
	if len(rs.reqs) != 1 || rs.reqs[0].ResumeID != "b" || m.Mode() != input.Chat {
		t.Fatalf("double click: %+v mode %v", rs.reqs, m.Mode())
	}
	m.HandleEvent(key("toggle"), t0)
	r = workspace.Compute(m.w, m.h, len(m.activeRows(t0)), false, false)
	m.HandleEvent(click(r.Chat.X+3, r.Chat.Y+3), t0.Add(time.Second))
	if m.Mode() != input.Chat {
		t.Fatalf("a click on the chat must focus it: %v", m.Mode())
	}
}

func TestMouseWheelMovesSelection(t *testing.T) {
	m, _, _ := workModel(t, []string{"/p"}, ent("a", "/p", "A", time.Hour))
	r := workspace.Compute(m.w, m.h, 0, false, false)
	m.treeKey = "f:/p"
	m.HandleEvent(input.Event{Kind: input.EvMouse, X: r.Tree.X + 2, Y: r.Tree.Y + 2, Btn: 65, Press: true}, t0)
	if m.treeKey != "s:a" {
		t.Fatalf("wheel down: %q", m.treeKey)
	}
	m.HandleEvent(input.Event{Kind: input.EvMouse, X: r.Tree.X + 2, Y: r.Tree.Y + 2, Btn: 64, Press: true}, t0)
	if m.treeKey != "f:/p" {
		t.Fatalf("wheel up: %q", m.treeKey)
	}
}

// recordAll runs a child that writes everything it receives to a file.
func recordAll(t *testing.T, prelude string) (Spawn, string) {
	out := filepath.Join(t.TempDir(), "got")
	return spawnID("rec", "sh", "-c", prelude+"stty raw -echo; : > "+out+"; exec cat >> "+out), out
}

func fileHas(path, want string) func() bool {
	return func() bool { b, _ := os.ReadFile(path); return string(b) == want }
}

func TestEscAndArrowsReachClaudeUntouched(t *testing.T) {
	spawn, out := recordAll(t, "")
	m := newModel(t, spawn)
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	time.Sleep(300 * time.Millisecond) // let the child switch to raw mode
	m.Input([]byte("\x1b"), t0)
	m.Input([]byte("\x1b[A"), t0)
	m.Input([]byte("abc\x1b[1;5C"), t0)
	waitFor(t, "bytes at the child", fileHas(out, "\x1b\x1b[Aabc\x1b[1;5C"))
}

func TestToggleLeavesChatOnceAndF1OpensHelpInChat(t *testing.T) {
	m := newModel(t, spawnCmd("sleep", "30"))
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	m.Input([]byte("\x00"), t0)
	if m.Mode() != input.Work {
		t.Fatalf("Ctrl+Space once must leave the chat: %v", m.Mode())
	}
	m.Input([]byte("\x00"), t0)
	if m.Mode() != input.Chat {
		t.Fatalf("Ctrl+Space again must come back: %v", m.Mode())
	}
	m.Input([]byte("\x1bOP"), t0)
	if m.Mode() != input.Overlay || m.overlay != ovHelp {
		t.Fatalf("F1 in chat: mode %v overlay %v", m.Mode(), m.overlay)
	}
	m.Input([]byte("\x1bOP"), t0)
	if m.Mode() != input.Chat {
		t.Fatalf("F1 again must close help and return to the chat: %v", m.Mode())
	}
}

func TestInputPasteInChatBypassesCommands(t *testing.T) {
	m := newModel(t, spawnID("p", "sh", "-c", `printf '\033[?2004h'; exec cat -v`))
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	waitFor(t, "mode 2004", m.Sessions()[0].BracketedPaste)
	m.Input([]byte("\x1b[200~a\x00b\x1b[201~"), t0)
	waitFor(t, "paste echoed", func() bool { return strings.Contains(m.Sessions()[0].Text(), "^[[200~a^@b^[[201~") })
	if m.Mode() != input.Chat {
		t.Fatal("a Ctrl+Space inside a paste must not act as the focus key")
	}
}

func TestInputPasteMarkersStrippedWhenChildLacksBracketedPaste(t *testing.T) {
	m := newModel(t, spawnID("p", "sh", "-c", "exec cat -v"))
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	m.Input([]byte("\x1b[200~hi\x1b[201~"), t0)
	waitFor(t, "hi echoed", func() bool { return strings.Contains(m.Sessions()[0].Text(), "hi") })
	if strings.Contains(m.Sessions()[0].Text(), "200~") {
		t.Fatalf("markers leaked to a child that did not ask for them: %q", m.Sessions()[0].Text())
	}
}

func TestInputPasteInWorkIsIgnored(t *testing.T) {
	m := newModel(t, spawnCmd("sleep", "30"))
	m.Input([]byte("\x1b[200~qqq\x1b[201~"), t0)
	if m.Quitting() {
		t.Fatal("pasted text acted as commands")
	}
	m.Input([]byte("q"), t0)
	m.Input([]byte("q"), t0)
	if !m.Quitting() {
		t.Fatal("normal keys must work again after the paste")
	}
}

func TestMouseInChatForwardedOnlyWhenWanted(t *testing.T) {
	spawn, out := recordAll(t, "")
	m := newModel(t, spawn)
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	time.Sleep(300 * time.Millisecond)
	r := workspace.Compute(m.w, m.h, 1, false, true)
	ev := input.Event{Kind: input.EvMouse, X: r.Chat.X + 2, Y: r.Chat.Y + 1, Press: true}
	m.HandleEvent(ev, t0)
	m.Input([]byte("z"), t0) // a marker that arrives after the mouse event would have
	waitFor(t, "only the marker", fileHas(out, "z"))

	spawn2, out2 := recordAll(t, `printf '\033[?1000h'; `)
	m2 := newModel(t, spawn2)
	m2.newSession(SpawnReq{Dir: "/p"}, t0)
	waitFor(t, "mouse on", m2.Sessions()[0].MouseWanted)
	time.Sleep(300 * time.Millisecond)
	m2.HandleEvent(ev, t0)
	waitFor(t, "translated click", fileHas(out2, "\x1b[<0;3;2M"))
}

func TestNewSessionOverlayAddsFolderToShown(t *testing.T) {
	m, rs, _ := workModel(t, nil)
	m.SetAccounts(map[string]string{"personal": "/cfg/.claude", "work": "/cfg/.claude-work"}, []string{"personal", "work"})
	m.HandleEvent(key("n"), t0)
	if m.Mode() != input.Overlay {
		t.Fatalf("mode %v", m.Mode())
	}
	for _, r := range "/tmp/x" {
		m.HandleEvent(key(string(r)), t0)
	}
	m.HandleEvent(key("enter"), t0)
	if len(rs.reqs) != 1 || rs.reqs[0] != (SpawnReq{Dir: "/tmp/x", ConfigDir: "/cfg/.claude"}) || m.Mode() != input.Chat {
		t.Fatalf("%+v mode %v", rs.reqs, m.Mode())
	}
	if !m.prefs.Shown("/tmp/x") {
		t.Fatal("a folder made with n must be shown from now on")
	}
}

func TestNewSessionOverlayRefusesMissingFolder(t *testing.T) {
	m, rs, _ := workModel(t, nil)
	m.statDir = func(string) bool { return false }
	m.HandleEvent(key("n"), t0)
	for _, r := range "/nope" {
		m.HandleEvent(key(string(r)), t0)
	}
	m.HandleEvent(key("enter"), t0)
	if len(rs.reqs) != 0 || m.Message(t0) == "" {
		t.Fatalf("spawns %d msg %q", len(rs.reqs), m.Message(t0))
	}
}

func TestFirstRunOpensFilterAndSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.json")
	rs := &recSpawn{}
	m := newModel(t, rs.spawn)
	m.SetPrefs(prefs.Load(path)) // never saved: first run
	m.SetIndex([]index.Entry{ent("a", "/p1", "", time.Hour), ent("b", "/p2", "", 2*time.Hour)})
	if m.Mode() != input.Overlay || m.overlay != ovFilter {
		t.Fatalf("first run: mode %v overlay %v", m.Mode(), m.overlay)
	}
	m.HandleEvent(key(" "), t0) // the first folder (newest: /p1) on
	m.HandleEvent(key("enter"), t0)
	if m.Mode() != input.Work || !prefs.Load(path).Shown("/p1") || prefs.Load(path).Shown("/p2") {
		t.Fatalf("mode %v; the choice must be saved", m.Mode())
	}
}

func TestFilterCancelDoesNotSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.json")
	m := newModel(t, spawnCmd("sleep", "30"))
	m.SetPrefs(prefs.Load(path))
	m.SetIndex([]index.Entry{ent("a", "/p1", "", time.Hour)})
	m.HandleEvent(key("esc"), t0)
	if _, err := os.Stat(path); err == nil {
		t.Fatal("cancelling the first-run filter must not write the file")
	}
}

func TestQueryFiltersTree(t *testing.T) {
	m, _, _ := workModel(t, []string{"/p"}, ent("a", "/p", "Referral", time.Hour), ent("b", "/p", "Lainnya", 2*time.Hour))
	m.HandleEvent(key("/"), t0)
	for _, r := range "ref" {
		m.HandleEvent(key(string(r)), t0)
	}
	m.HandleEvent(key("enter"), t0)
	rows := m.treeRows(t0)
	if m.query != "ref" || len(rows) != 2 || rows[1].ID != "a" {
		t.Fatalf("query %q rows %+v", m.query, rows)
	}
	m.HandleEvent(key("esc"), t0)
	if m.query != "" {
		t.Fatal("esc must clear the query")
	}
}

func TestQuitAsksWhenSessionsAlive(t *testing.T) {
	m := newModel(t, spawnCmd("sleep", "30"))
	m.HandleEvent(key("q"), t0)
	if !m.Quitting() {
		t.Fatal("nothing runs: q must quit at once")
	}
	m = newModel(t, spawnCmd("sleep", "30"))
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	m.HandleEvent(key("toggle"), t0)
	m.HandleEvent(key("q"), t0)
	if m.Quitting() || !strings.Contains(m.Message(t0), "q lagi") {
		t.Fatalf("first q: quitting %v msg %q", m.Quitting(), m.Message(t0))
	}
	m.HandleEvent(key("q"), t0.Add(time.Second))
	if !m.Quitting() {
		t.Fatal("second q must quit")
	}
}

func TestMaxNineSessions(t *testing.T) {
	m := newModel(t, spawnCmd("sleep", "30"))
	for i := 0; i < 9; i++ {
		m.newSession(SpawnReq{Dir: "/p"}, t0)
	}
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	if len(m.Sessions()) != 9 || !strings.Contains(m.Message(t0), "maksimum 9") {
		t.Fatalf("sessions %d msg %q", len(m.Sessions()), m.Message(t0))
	}
}

func TestSpawnErrorKeepsAppAlive(t *testing.T) {
	m := newModel(t, func(SpawnReq, int, int) (*session.Session, error) { return nil, os.ErrNotExist })
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	if len(m.Sessions()) != 0 || m.Mode() != input.Work || !strings.Contains(m.Message(t0), "gagal memulai") {
		t.Fatalf("sessions %d mode %v msg %q", len(m.Sessions()), m.Mode(), m.Message(t0))
	}
}

func TestMessageExpiresAfterThreeSeconds(t *testing.T) {
	m := newModel(t, spawnCmd("sleep", "30"))
	m.say(t0, "halo")
	if m.Message(t0) != "halo" || m.Message(t0.Add(4*time.Second)) != "" {
		t.Fatal("message should expire after 3s")
	}
}

func TestCloseOnlyRemovesExited(t *testing.T) {
	spawns := []Spawn{spawnID("run", "sleep", "30"), spawnID("dead", "sh", "-c", "exit 0")}
	i := 0
	m := newModel(t, func(r SpawnReq, w, h int) (*session.Session, error) { i++; return spawns[i-1](r, w, h) })
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	waitFor(t, "exit", m.Sessions()[1].Exited)
	m.HandleEvent(key("toggle"), t0) // leave the chat; the dead session is the selected one
	m.HandleEvent(key("x"), t0)
	if len(m.Sessions()) != 1 || m.Sessions()[0].ID() != "run" {
		t.Fatalf("sessions %d", len(m.Sessions()))
	}
	m.pane, m.activeKey = workspace.PaneActive, "run"
	m.HandleEvent(key("x"), t0)
	if len(m.Sessions()) != 1 || !strings.Contains(m.Message(t0), "masih berjalan") {
		t.Fatalf("a running session must not be closed: %q", m.Message(t0))
	}
}

func TestResizeAppliesChatSizeToAllSessions(t *testing.T) {
	m := newModel(t, spawnCmd("sh", "-c", "sleep 0.4; stty size; sleep 5"))
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	m.Resize(100, 30) // left column 28, chat 72 wide, 26 high
	waitFor(t, "stty size 26 72", func() bool { return strings.Contains(m.Sessions()[0].Text(), "26 72") })
}

func TestFrameShowsLimits(t *testing.T) {
	m := newModel(t, spawnCmd("sleep", "30"))
	m.SetLimits([]limits.Usage{{Account: "personal", FiveHour: limits.Window{Pct: 78, ResetsAt: t0}, SevenDay: limits.Window{Pct: 39}}})
	rows := plainRows(Frame(m, 120, 30, t0), 30)
	if !strings.Contains(rows[29], "5j 78%") || !strings.Contains(rows[29], "7h 39%") {
		t.Fatalf("%q", rows[29])
	}
	m.SetLimits([]limits.Usage{{Account: "personal", Err: "offline"}})
	if rows := plainRows(Frame(m, 120, 30, t0), 30); !strings.Contains(rows[29], "offline") {
		t.Fatalf("%q", rows[29])
	}
}

// two PTYs: "run" alive (index 0), "dead" exited (index 1, the focus after opening it last)
func twoSessionModel(t *testing.T) *Model {
	t.Helper()
	spawns := []Spawn{spawnID("run", "sleep", "30"), spawnID("dead", "sh", "-c", "exit 0")}
	i := 0
	m := newModel(t, func(r SpawnReq, w, h int) (*session.Session, error) { i++; return spawns[i-1](r, w, h) })
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	waitFor(t, "exit", m.Sessions()[1].Exited)
	m.HandleEvent(key("toggle"), t0)
	m.pane, m.activeKey = workspace.PaneActive, "run"
	return m
}

func TestReturningToChatFollowsTheShownSession(t *testing.T) {
	enter := map[string]func(m *Model){
		"ctrl+space": func(m *Model) { m.HandleEvent(key("toggle"), t0) },
		"z":          func(m *Model) { m.HandleEvent(key("z"), t0) },
		"click": func(m *Model) {
			r := workspace.Compute(m.w, m.h, len(m.activeRows(t0)), false, false)
			m.HandleEvent(click(r.Chat.X+3, r.Chat.Y+3), t0)
		},
	}
	for name, f := range enter {
		m := twoSessionModel(t)
		f(m)
		if m.Mode() != input.Chat || m.FocusIndex() != 0 {
			t.Errorf("%s: mode %v focus %d, want the chat of the live session (index 0)", name, m.Mode(), m.FocusIndex())
		}
	}
}

func TestLeavingChatClearsZoom(t *testing.T) {
	m := twoSessionModel(t)
	m.HandleEvent(key("z"), t0)
	if !m.zoom {
		t.Fatal("z did not zoom")
	}
	m.HandleEvent(click(5, m.h-1), t0) // a click outside the zoomed chat leaves it
	if m.Mode() != input.Work || m.zoom {
		t.Fatalf("mode %v zoom %v: the lists must be visible again", m.Mode(), m.zoom)
	}
	m.HandleEvent(key("z"), t0)
	m.HandleEvent(key("f1"), t0) // help over the zoomed chat, whose session then exits
	m.Sessions()[0].Close()
	waitFor(t, "exit", m.Sessions()[0].Exited)
	m.HandleEvent(key("esc"), t0)
	if m.Mode() != input.Work || m.zoom {
		t.Fatalf("closing help: mode %v zoom %v", m.Mode(), m.zoom)
	}
}

func TestNewSessionPathIsCleaned(t *testing.T) {
	m, rs, _ := workModel(t, nil)
	m.HandleEvent(key("n"), t0)
	for _, r := range "/tmp/x/../y/" {
		m.HandleEvent(key(string(r)), t0)
	}
	m.HandleEvent(key("enter"), t0)
	if len(rs.reqs) != 1 || rs.reqs[0].Dir != "/tmp/y" || !m.prefs.Shown("/tmp/y") {
		t.Fatalf("%+v: a trailing slash or .. must not make the folder differ from the transcript's cwd", rs.reqs)
	}
}

func TestAKeyCyclesTheAccountFilter(t *testing.T) {
	w := ent("w", "/p", "kerja", time.Hour)
	w.Account = "work"
	m, _, _ := workModel(t, []string{"/p"}, w, ent("p", "/p", "pribadi", 2*time.Hour))
	m.SetAccounts(nil, []string{"personal", "work"})
	ids := func() (s string) {
		for _, r := range m.treeRows(t0) {
			s += r.ID
		}
		return
	}
	for _, want := range []string{"p", "w", "wp"} {
		m.HandleEvent(key("a"), t0)
		if got := ids(); got != want {
			t.Fatalf("filter %q: tree %q want %q", m.account, got, want)
		}
	}
}
