// Package app holds nakama's state (sessions, panels, focus, mode) and renders it.
package app

import (
	"fmt"
	"image"
	"math/rand/v2"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/kalyskhairy/nakama/internal/history"
	"github.com/kalyskhairy/nakama/internal/index"
	"github.com/kalyskhairy/nakama/internal/input"
	"github.com/kalyskhairy/nakama/internal/limits"
	"github.com/kalyskhairy/nakama/internal/live"
	"github.com/kalyskhairy/nakama/internal/monitor"
	"github.com/kalyskhairy/nakama/internal/office"
	"github.com/kalyskhairy/nakama/internal/prefs"
	"github.com/kalyskhairy/nakama/internal/scene"
	"github.com/kalyskhairy/nakama/internal/session"
	"github.com/kalyskhairy/nakama/internal/theme"
	"github.com/kalyskhairy/nakama/internal/ui"
	"github.com/kalyskhairy/nakama/internal/workspace"
)

const (
	maxSessions = 9
	msgTTL      = 3 * time.Second
	confirmTTL  = 5 * time.Second
	dblClick    = 400 * time.Millisecond
)

// SpawnReq says what to start: a new session in Dir, or ResumeID resumed in Dir. ConfigDir is the
// config dir of the account that owns the session ("" for the personal account).
type SpawnReq struct{ Dir, ConfigDir, ResumeID string }

type Spawn func(r SpawnReq, w, h int) (*session.Session, error)

// Monitor reports what the transcripts say about a session, by Claude session id.
type Monitor interface {
	Poll(sessionID string, now time.Time)
	Info(sessionID string, now time.Time) monitor.Info
	Subagents(sessionID string, now time.Time) []monitor.SubInfo
	Activity(sessionID string) []monitor.Activity
}

type noMonitor struct{}

func (noMonitor) Poll(string, time.Time)                        {}
func (noMonitor) Info(string, time.Time) monitor.Info           { return monitor.Info{CtxPct: -1} }
func (noMonitor) Subagents(string, time.Time) []monitor.SubInfo { return nil }
func (noMonitor) Activity(string) []monitor.Activity            { return nil }

type officeView struct {
	th *theme.Theme
	r  *office.Renderer
}

// OfficeConfig is the office screen. A non-empty Reason means it cannot be shown (no theme,
// no sixel, ...); `v` then says why.
type OfficeConfig struct {
	Theme        *theme.Theme
	Renderer     *office.Renderer
	CellW, CellH int
	MaxScale     float64 // cap on the picture scale; 0 means 1
	CellKnown    bool    // CellW x CellH come from the terminal, so parts of the picture can be placed
	Reason       string
}

type sessMeta struct{ dir, configDir, account string }

type overlayKind int

const (
	ovNone overlayKind = iota
	ovHelp
	ovFilter
	ovNew
	ovDetail
	ovUsage
	ovSearch
)

type Model struct {
	spawn    Spawn
	w, h     int
	sessions []*session.Session // the PTYs nakama runs, at most maxSessions
	meta     map[*session.Session]sessMeta
	sized    map[*session.Session][2]int
	focus    int // index in sessions of the one in the chat panel; -1 none
	mode     input.Mode
	toggle   byte

	msg         string
	msgUntil    time.Time
	quitAskedAt time.Time
	quitting    bool
	inPaste     bool

	mon    Monitor
	limits []limits.Usage

	// the three panels
	entries      []index.Entry
	procs        []live.Proc
	prefs        *prefs.Folders
	accounts     map[string]string // account name -> config dir
	accountNames []string
	defaultDir   string
	prompted     bool // the first-run folder filter has been offered
	pane         workspace.Pane
	activeKey    string // selected session id in Aktif
	treeKey      string // "f:<cwd>" or "s:<id>" in Folder/Sesi
	expanded     map[string]bool
	query        string
	querying     bool
	account      string // account the tree is filtered to; "" means all
	zoom         bool
	mouseOn      bool
	mouseSent    bool
	confirmID    string
	confirmAt    time.Time
	lastClick    time.Time
	lastX, lastY int
	statDir      func(string) bool

	// overlays
	inOffice   bool // the chat is the office's panel, not the three panels'
	lastImage  time.Time
	lastTyped  time.Time // last keystroke sent to the chat
	imgSig     string    // what the last picture showed
	overlay    overlayKind
	prevMode   input.Mode
	helpScroll int
	filter     *workspace.FilterPicker
	newPick    *workspace.NewPicker
	detailID   string

	// the office
	office     *OfficeConfig
	views      []officeView // the theme as drawn, and turned upright
	view       int
	world      *scene.World
	actors     []scene.Actor
	crewArea   workspace.Rect  // the crew list on the office screen
	crewIDs    []string        // the agent of each of its lines under the header
	roots      func() []string // transcript roots, for usage and search
	background func(work func() func())
	report     *history.Report
	hits       []history.Hit
	hit        int
	searching  bool
	searchSeq  int
	// what is typed, and what the hits are for
	searchQuery, searched string
	order                 []string          // actor ids as the crew list shows them: what Tab and the arrows walk
	owner                 map[string]string // actor id -> id of the session it belongs to
	cards                 map[string]card
	lastPic               *image.NRGBA      // the office picture on the screen
	shipLay               office.Layout     // and where it is: for clicks
	seen                  map[string]*watch // session id -> what notices are judged against
	notices               []Notice
	cast                  map[string]string // session id -> character, kept for as long as nakama runs
	selected              string
	cardOpen              bool
	shipOnly              bool   // the office shows the ship alone, filling the screen
	drewOffice            bool   // the screen currently holds the office picture
	lastKey               string // layout of the last office frame; a change clears the screen
	lastSig               string // everything the last office frame showed; unchanged means nothing is sent
}

var (
	pasteStart = []byte("\x1b[200~")
	pasteEnd   = []byte("\x1b[201~")
)

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func NewModel(spawn Spawn, w, h int) *Model {
	return &Model{spawn: spawn, w: w, h: h, focus: -1, mode: input.Work, mon: noMonitor{}, pane: workspace.PaneTree,
		meta: map[*session.Session]sessMeta{}, sized: map[*session.Session][2]int{}, expanded: map[string]bool{},
		prefs: prefs.Load(""), accounts: map[string]string{}, mouseOn: true, statDir: isDir}
}

func (m *Model) Mode() input.Mode             { return m.mode }
func (m *Model) Sessions() []*session.Session { return m.sessions }
func (m *Model) FocusIndex() int              { return m.focus }
func (m *Model) Quitting() bool               { return m.quitting }
func (m *Model) MouseOn() bool                { return m.mouseOn }

// MouseSeq is the escape sequence that makes the terminal's mouse reporting match MouseOn, or ""
// when it already does. The caller writes it before the frame.
func (m *Model) MouseSeq() string {
	if m.mouseOn == m.mouseSent {
		return ""
	}
	m.mouseSent = m.mouseOn
	if m.mouseOn {
		return "\x1b[?1000h\x1b[?1006h"
	}
	return "\x1b[?1000l\x1b[?1006l"
}

// SetToggle sets the byte of the key that moves focus between the chat and the lists (0 = Ctrl+Space).
func (m *Model) SetToggle(b byte) { m.toggle = b }

func (m *Model) SetPrefs(p *prefs.Folders) { m.prefs = p }

// SetAccounts says which config dir belongs to each account name, personal first.
func (m *Model) SetAccounts(byName map[string]string, order []string) {
	m.accounts, m.accountNames = byName, order
}

// SetDefaultDir is the folder the new-session picker offers when no session ran there yet.
func (m *Model) SetDefaultDir(dir string) { m.defaultDir = dir }

// SetIndex replaces the list of sessions on the machine. The first time it has sessions and the
// owner never chose folders, the folder filter opens.
func (m *Model) SetIndex(es []index.Entry) {
	m.entries = es
	if len(m.expanded) == 0 && len(es) > 0 {
		m.expanded[es[0].Cwd] = true
	}
	if !m.prompted && len(es) > 0 && m.prefs.First() && m.mode != input.Overlay {
		m.prompted = true
		m.openFilter()
	}
}

func (m *Model) SetLive(ps []live.Proc) {
	before := make([]string, len(m.sessions))
	for i, s := range m.sessions {
		before[i] = m.ownedID(s)
	}
	m.procs = ps
	for i, s := range m.sessions {
		if id := m.ownedID(s); id != before[i] {
			m.renameSession(before[i], id)
		}
	}
}

// renameSession moves what is kept under a session id when Claude gives the session a new one (/clear,
// a resume that forks), so the selection and the character stay with it.
func (m *Model) renameSession(old, id string) {
	if m.activeKey == old {
		m.activeKey = id
	}
	if m.treeKey == "s:"+old {
		m.treeKey = "s:" + id
	}
	if m.selected == old {
		m.selected = id
	}
	if c := m.cast[old]; c != "" {
		m.cast[id] = c
		delete(m.cast, old)
	}
}

func (m *Model) Message(now time.Time) string {
	if now.After(m.msgUntil) {
		return ""
	}
	return m.msg
}

func (m *Model) say(now time.Time, format string, a ...any) {
	m.msg, m.msgUntil = fmt.Sprintf(format, a...), now.Add(msgTTL)
}

func sessionKey(s *session.Session) string {
	if s.ID() != "" {
		return s.ID()
	}
	return fmt.Sprintf("pty-%p", s)
}

func untracked(id string) bool { return strings.HasPrefix(id, "pty-") }

// ownedID is the Claude session id a PTY runs now. A resumed session or a /clear can get a new id, which
// Claude records under the process id, so that wins over the id the session started with.
func (m *Model) ownedID(s *session.Session) string {
	if p, ok := live.ByPID(m.procs, s.PID()); ok {
		return p.SessionID
	}
	return sessionKey(s)
}

func (m *Model) ownedAll() map[string]*session.Session {
	out := make(map[string]*session.Session, len(m.sessions))
	for _, s := range m.sessions {
		out[m.ownedID(s)] = s
	}
	return out
}

func (m *Model) allEntries(now time.Time) []index.Entry {
	have := make(map[string]bool, len(m.entries))
	for _, e := range m.entries {
		have[e.ID] = true
	}
	var out []index.Entry
	for _, s := range m.sessions { // a session that has no transcript yet still shows
		if id := m.ownedID(s); !have[id] {
			md := m.meta[s]
			out = append(out, index.Entry{ID: id, Account: md.account, ConfigDir: md.configDir, Cwd: md.dir, Title: s.Title(), ModTime: now})
		}
	}
	return append(out, m.entries...)
}

func (m *Model) info(id string, now time.Time) monitor.Info {
	if untracked(id) {
		return monitor.Info{CtxPct: -1}
	}
	in := m.mon.Info(id, now)
	for _, p := range m.procs { // the transcript shows no work while Claude thinks or writes; its own record does
		if p.SessionID == id && p.Status == "busy" && in.Status != monitor.Waiting {
			in.Status = monitor.Working
		}
	}
	return in
}

// waitingSubs counts the subagents of a session that wait for approval. The lead itself only looks
// busy then (its pending tool is the Agent call), so they are counted on their own.
func (m *Model) waitingSubs(id string, now time.Time) int {
	if untracked(id) {
		return 0
	}
	n := 0
	for _, sub := range m.mon.Subagents(id, now) {
		if sub.Info.Status == monitor.Waiting {
			n++
		}
	}
	return n
}

func (m *Model) source(now time.Time) workspace.Source {
	owned := m.ownedAll()
	alive := map[string]bool{}
	procs := live.BySession(m.procs)
	for id, s := range owned {
		if !s.Exited() {
			alive[id] = true
		} else if p, ok := procs[id]; ok && p.PID == s.PID() {
			delete(procs, id) // our own, read just before it ended
		}
	}
	return workspace.Source{TreeAccount: m.account, Entries: m.allEntries(now), Live: procs, Owned: alive, Now: now,
		State: func(id string) ui.State {
			if s := owned[id]; s != nil && s.Exited() {
				return ui.StateExited
			}
			in := m.info(id, now)
			switch {
			case in.Status == monitor.Waiting || m.waitingSubs(id, now) > 0:
				return ui.StateWaiting
			case in.Status == monitor.Working:
				return ui.StateWorking
			case in.Status == monitor.Active:
				return ui.StateActive
			}
			return ui.StateIdle
		}}
}

func (m *Model) activeRows(now time.Time) []workspace.ActiveRow {
	return workspace.BuildActive(m.source(now))
}

func (m *Model) treeRows(now time.Time) []workspace.TreeRow {
	return workspace.BuildTree(m.source(now), m.prefs, m.expanded, m.query)
}

// Poll lets the monitor read the transcripts that matter now: Aktif, the selected session, and ours.
func (m *Model) Poll(now time.Time) {
	seen := map[string]bool{}
	poll := func(id string) {
		if id != "" && !untracked(id) && !seen[id] {
			seen[id] = true
			m.mon.Poll(id, now)
		}
	}
	for _, r := range m.activeRows(now) {
		poll(r.ID)
	}
	poll(m.selectedID(now))
	for _, s := range m.sessions {
		poll(m.ownedID(s))
	}
	m.watchCrew(now)
}

// Summary is what the status line shows: who waits, the plan limits, tokens of the sessions in Aktif.
func (m *Model) Summary(now time.Time) ui.Summary {
	var sum ui.Summary
	for _, r := range m.activeRows(now) {
		if r.State == ui.StateExited {
			continue
		}
		in := m.info(r.ID, now)
		if in.Status == monitor.Waiting {
			sum.Waiting++
		}
		sum.Waiting += m.waitingSubs(r.ID, now)
		sum.TokensToday += in.TokensToday
	}
	for _, l := range m.limits {
		v := ui.LimitView{Account: l.Account, FivePct: -1, SevenPct: -1, Err: l.Err}
		if l.Err == "" {
			v.FivePct, v.SevenPct = int(l.FiveHour.Pct+0.5), int(l.SevenDay.Pct+0.5)
			v.FiveReset, v.SevenReset = resetText(l.FiveHour.ResetsAt, now), resetText(l.SevenDay.ResetsAt, now)
		}
		sum.Limits = append(sum.Limits, v)
	}
	return sum
}

func (m *Model) SetMonitor(mon Monitor)     { m.mon = mon }
func (m *Model) SetLimits(l []limits.Usage) { m.limits = l }

func resetText(at, now time.Time) string {
	if at.IsZero() {
		return ""
	}
	at = at.In(now.Location())
	if at.Sub(now) > 24*time.Hour {
		return at.Format("Mon 15:04")
	}
	return at.Format("15:04")
}

// ---- sizes

// chatSize is the size every PTY gets: the chat area of the current layout.
func (m *Model) chatSize(now time.Time) (int, int) {
	if l, ok := m.officeLayout(m.w, m.h); ok && m.inOffice {
		return l.chat.W, l.chat.H
	}
	r := workspace.Compute(m.w, m.h, len(m.activeRows(now)), m.zoom, m.mode == input.Chat)
	if r.Chat.W < 1 || r.Chat.H < 1 { // narrow screen with the lists showing: the size it will have when zoomed
		r = workspace.Compute(m.w, m.h, 0, true, true)
	}
	return max(r.Chat.W, 1), max(r.Chat.H, 1)
}

// syncSizes gives every session the size of the chat area, if it changed.
func (m *Model) syncSizes(now time.Time) {
	w, h := m.chatSize(now)
	for _, s := range m.sessions {
		if m.sized[s] != [2]int{w, h} {
			s.Resize(w, h)
			m.sized[s] = [2]int{w, h}
		}
	}
}

func (m *Model) Resize(w, h int) {
	m.w, m.h = w, h
	if _, ok := m.officeLayout(w, h); !ok {
		m.inOffice = false // no room for the panel any more: the chat takes the screen
	}
	m.orient()
	m.syncSizes(time.Now())
}

// ---- sessions

func (m *Model) alive() int {
	n := 0
	for _, s := range m.sessions {
		if !s.Exited() {
			n++
		}
	}
	return n
}

// newSession starts a PTY and opens its chat.
func (m *Model) newSession(req SpawnReq, now time.Time) bool {
	if len(m.sessions) >= maxSessions {
		m.say(now, "at most %d sessions", maxSessions)
		return false
	}
	w, h := m.chatSize(now)
	s, err := m.spawn(req, w, h)
	if err != nil {
		m.say(now, "could not start the session: %v", err)
		return false
	}
	acct := "personal"
	if req.ConfigDir != "" {
		acct = limits.AccountName(req.ConfigDir)
	}
	m.sessions = append(m.sessions, s)
	m.meta[s], m.sized[s] = sessMeta{dir: req.Dir, configDir: req.ConfigDir, account: acct}, [2]int{w, h}
	m.openOwned(len(m.sessions)-1, now)
	return true
}

// openOwned shows session i in the chat panel and gives it the focus.
func (m *Model) openOwned(i int, now time.Time) {
	m.focus, m.mode = i, input.Chat
	id := m.ownedID(m.sessions[i])
	if m.inOffice {
		m.selected = id
	}
	m.pane, m.activeKey, m.treeKey = workspace.PaneActive, id, "s:"+id
	if len(m.activeRows(now)) == 0 {
		m.pane = workspace.PaneTree
	}
}

func (m *Model) removeSession(i int) {
	s := m.sessions[i]
	s.Close()
	delete(m.meta, s)
	delete(m.sized, s)
	m.sessions = append(m.sessions[:i], m.sessions[i+1:]...)
	switch {
	case m.focus == i:
		m.focus = -1
	case m.focus > i:
		m.focus--
	}
}

func (m *Model) quit(now time.Time) {
	n := m.alive()
	if n == 0 || (!m.quitAskedAt.IsZero() && now.Sub(m.quitAskedAt) <= msgTTL) {
		m.quitting = true
		return
	}
	m.quitAskedAt = now
	m.say(now, "%d sessions still running, press q again to quit", n)
}

// ---- the office

func (m *Model) officeActive() bool {
	return m.office != nil && m.office.Reason == "" && m.office.Renderer != nil && m.world != nil
}

func (m *Model) SetOffice(cfg *OfficeConfig) {
	m.office, m.world, m.views, m.view = cfg, nil, nil, 0
	if cfg == nil || cfg.Reason != "" || cfg.Renderer == nil || cfg.Theme == nil {
		return
	}
	m.world, m.views = scene.NewWorld(cfg.Theme), []officeView{{cfg.Theme, cfg.Renderer}}
	up := theme.Portrait(cfg.Theme)
	if r, err := office.NewRenderer(up); err == nil {
		m.views = append(m.views, officeView{up, r})
	}
	m.orient()
}

// orient picks the way up that gives the bigger picture. Turning starts a new world, so the crew
// walks in again.
func (m *Model) orient() {
	if !m.officeActive() || len(m.views) < 2 {
		return
	}
	cols, rows := m.w, m.h-2
	if l, ok := m.officeLayout(m.w, m.h); ok {
		cols, rows = l.shipCols, l.shipRows
	}
	area := func(v officeView) int {
		b := v.th.Background.Bounds()
		l, ok := office.FitMax(cols, rows, m.office.CellW, m.office.CellH, b.Dx(), b.Dy(), m.maxScale())
		if !ok {
			return 0
		}
		return l.ImgW * l.ImgH
	}
	best := 0
	if area(m.views[1]) > area(m.views[0]) {
		best = 1
	}
	if best != m.view {
		m.view, m.world, m.lastKey = best, scene.NewWorld(m.views[best].th), ""
	}
}

// shipOnlyScale is how far the ship may be enlarged when it is shown alone.
const shipOnlyScale = 2

func (m *Model) maxScale() float64 {
	s := m.office.MaxScale
	if s <= 0 {
		s = 1
	}
	if m.shipOnly {
		s = max(s, shipOnlyScale)
	}
	return s
}

func (m *Model) officeShown() bool {
	return m.officeActive() && (m.mode == input.Home || (m.mode == input.Chat && m.inOffice))
}

// Animating reports whether the screen shows the office, which needs frequent redraws.
func (m *Model) Animating() bool { return m.officeShown() }

// Typing reports a chat panel with the focus in the office: its keystrokes must not wait for the
// next picture.
func (m *Model) Typing() bool { return m.mode == input.Chat && m.inOffice }

// SetCellSize updates the terminal cell size in pixels (it changes with the font size).
func (m *Model) SetCellSize(cw, ch int) {
	if m.office != nil && cw > 0 && ch > 0 {
		m.office.CellW, m.office.CellH, m.office.CellKnown = cw, ch, true
		m.orient()
	}
}

func (m *Model) Selected() (string, bool) { return m.selected, m.selected != "" }

func (m *Model) Actors() []scene.Actor { return append([]scene.Actor(nil), m.actors...) }

func sceneStatus(in monitor.Status, exited bool) scene.Status {
	switch {
	case exited:
		return scene.Exited
	case in == monitor.Working:
		return scene.Working
	case in == monitor.Waiting:
		return scene.Waiting
	case in == monitor.Active:
		return scene.Active
	}
	return scene.Idle
}

var statusText = map[scene.Status]string{scene.Working: "working", scene.Active: "active", scene.Idle: "idle", scene.Waiting: "needs approval", scene.Exited: "done"}

// card is what is known about one agent.
type card struct {
	title, role string
	facts       []string // "model: x", "status: x", ...
}

// rows is the card squeezed into three screen-wide lines, for under the picture.
func (c card) rows() [3]string {
	l1, l2 := " "+c.title+" · "+c.role, " "
	for _, f := range c.facts {
		if m, ok := strings.CutPrefix(f, "model: "); ok {
			l1 += " · " + m
			continue
		}
		if st, ok := strings.CutPrefix(f, "status: "); ok {
			f = st
		}
		if l2 != " " {
			l2 += " · "
		}
		l2 += strings.Replace(f, "context: ", "context ", 1)
	}
	return [3]string{l1, l2, " Enter chat · i close card · Tab select · v back"}
}

func newCard(title, role string, st scene.Status, in monitor.Info) card {
	c := card{title: title, role: role}
	if in.Model != "" {
		c.facts = append(c.facts, "model: "+strings.TrimPrefix(in.Model, "claude-"))
	}
	ctx := "?"
	if in.CtxPct >= 0 {
		ctx = fmt.Sprintf("%d%%", in.CtxPct)
	}
	c.facts = append(c.facts, "status: "+statusText[st], "context: "+ctx)
	if in.PendingTool != "" && (st == scene.Working || st == scene.Waiting) {
		c.facts = append(c.facts, "tool: "+in.PendingTool)
	}
	return c
}

// crewRows is who is aboard: the active sessions, without the ones that have ended.
func (m *Model) crewRows(now time.Time) []workspace.ActiveRow {
	var out []workspace.ActiveRow
	for _, r := range m.activeRows(now) {
		if r.State != ui.StateExited {
			out = append(out, r)
		}
	}
	return out
}

// castCrew gives every session its own character. The first character of the list is the captain:
// whenever nobody aboard is it, the first newcomer (the session on top of the list, if new) becomes
// it. The others are drawn at random from the least worn ones.
func (m *Model) castCrew(rows []workspace.ActiveRow, first string) {
	pool := m.office.Theme.SessionCharacters
	if len(pool) == 0 {
		return
	}
	if m.cast == nil {
		m.cast = map[string]string{}
	}
	worn := map[string]int{}
	var newcomers []string
	for _, r := range rows {
		switch c := m.cast[r.ID]; {
		case c != "":
			worn[c]++
		case r.ID == first:
			newcomers = append([]string{r.ID}, newcomers...)
		default:
			newcomers = append(newcomers, r.ID)
		}
	}
	for _, id := range newcomers {
		pick := pool[0]
		if worn[pick] > 0 {
			var least []string
			for _, c := range pool[min(1, len(pool)-1):] {
				if len(least) == 0 || worn[c] < worn[least[0]] {
					least = []string{c}
				} else if worn[c] == worn[least[0]] {
					least = append(least, c)
				}
			}
			pick = least[rand.IntN(len(least))]
		}
		m.cast[id] = pick
		worn[pick]++
	}
}

// Tick refreshes the office: who is there (every active session and its subagents) and where they walk.
func (m *Model) Tick(now time.Time) {
	if !m.officeActive() {
		return
	}
	var actors []scene.Actor
	owner := map[string]string{}
	cards := map[string]card{}
	rows := m.crewRows(now)
	first := ""
	if len(rows) > 0 {
		first = rows[0].ID // the top of the list, before it is put in a fixed order
	}
	listed := map[string]int{} // place in the crew list
	for i, r := range rows {
		listed[r.ID] = i
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID }) // a status change must not reshuffle the seats
	m.castCrew(rows, first)
	for _, r := range rows {
		key := r.ID
		in := m.info(key, now)
		st := sceneStatus(in.Status, false)
		actors = append(actors, scene.Actor{ID: key, Type: "session", Title: r.Title, Status: st, LastActive: in.LastActive, Lead: true, Character: m.cast[key], Visit: m.visiting(key, in, now), Full: in.CtxPct >= ctxWarn})
		role := "session lead"
		if name := charName(m.cast[key]); name != "" {
			role += " · " + name
		}
		owner[key], cards[key] = key, newCard(r.Title, role, st, in)
		if untracked(key) {
			continue
		}
		for _, sub := range m.mon.Subagents(key, now) {
			id, sst := "sub:"+key+":"+sub.ID, sceneStatus(sub.Info.Status, false)
			title := sub.Description
			if title == "" {
				title = sub.Type
			}
			actors = append(actors, scene.Actor{ID: id, Type: sub.Type, Title: title, Status: sst, LastActive: sub.LastActive, Character: m.cast[key]})
			owner[id], cards[id] = key, newCard(title, "subagent "+sub.Type, sst, sub.Info)
		}
	}
	order := make([]string, len(actors))
	for i, a := range actors {
		order[i] = a.ID
	}
	sort.SliceStable(order, func(i, j int) bool { return listed[owner[order[i]]] < listed[owner[order[j]]] })
	// keep the selection on the same actor; if it left, move to its neighbour
	if m.selected != "" {
		if _, ok := owner[m.selected]; !ok {
			prev := 0
			for i, id := range m.order {
				if id == m.selected {
					prev = i
				}
			}
			m.selected = ""
			if len(order) > 0 {
				m.selected = order[min(prev, len(order)-1)]
			}
		}
	}
	m.actors, m.order, m.owner, m.cards = actors, order, owner, cards
	m.world.Update(actors, now)
}

func (m *Model) selectStep(d int) {
	n := len(m.order)
	if n == 0 {
		return
	}
	idx := -1
	for i, id := range m.order {
		if id == m.selected {
			idx = i
		}
	}
	switch {
	case idx < 0 && d > 0:
		idx = 0
	case idx < 0:
		idx = n - 1
	default:
		idx = (idx + d + n) % n
	}
	m.selected = m.order[idx]
}
