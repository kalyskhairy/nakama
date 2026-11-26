package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kalyskhairy/nakama/internal/index"
	"github.com/kalyskhairy/nakama/internal/input"
	"github.com/kalyskhairy/nakama/internal/session"
	"github.com/kalyskhairy/nakama/internal/workspace"
)

func rowKey(r workspace.TreeRow) string {
	if r.Folder {
		return "f:" + r.Cwd
	}
	return "s:" + r.ID
}

// selection returns both panels' rows and the index of the selected row in each.
func (m *Model) selection(now time.Time) (active []workspace.ActiveRow, ai int, tree []workspace.TreeRow, ti int) {
	active, tree = m.activeRows(now), m.treeRows(now)
	for i, r := range active {
		if r.ID == m.activeKey {
			ai = i
		}
	}
	for i, r := range tree {
		if rowKey(r) == m.treeKey {
			ti = i
		}
	}
	return
}

// selectedID is the session the strip and the chat panel are about ("" on a folder or nothing).
func (m *Model) selectedID(now time.Time) string {
	if m.mode == input.Chat && m.focus >= 0 && m.focus < len(m.sessions) {
		return m.ownedID(m.sessions[m.focus])
	}
	active, ai, tree, ti := m.selection(now)
	if m.pane == workspace.PaneActive {
		if len(active) > 0 {
			return active[ai].ID
		}
		return ""
	}
	if len(tree) > 0 && !tree[ti].Folder {
		return tree[ti].ID
	}
	return ""
}

// shown is the PTY in the chat panel: the one of the selected session, if nakama runs it.
func (m *Model) shown(now time.Time) *session.Session {
	if id := m.selectedID(now); id != "" {
		return m.ownedAll()[id]
	}
	return nil
}

func (m *Model) move(d int, now time.Time) {
	active, ai, tree, ti := m.selection(now)
	if m.pane == workspace.PaneActive {
		if n := len(active); n > 0 {
			m.activeKey = active[max(0, min(ai+d, n-1))].ID
		}
		return
	}
	if n := len(tree); n > 0 {
		m.treeKey = rowKey(tree[max(0, min(ti+d, n-1))])
	}
}

// cycleAccount steps the tree through: all accounts, then each account.
func (m *Model) cycleAccount(now time.Time) {
	next := ""
	if m.account == "" && len(m.accountNames) > 0 {
		next = m.accountNames[0]
	}
	for i, a := range m.accountNames {
		if a == m.account && i+1 < len(m.accountNames) {
			next = m.accountNames[i+1]
		}
	}
	m.account = next
	if next == "" {
		next = "semua"
	}
	m.say(now, "akun di pohon: %s", next)
}

func (m *Model) entry(now time.Time, id string) (index.Entry, bool) {
	for _, e := range m.allEntries(now) {
		if e.ID == id {
			return e, true
		}
	}
	return index.Entry{}, false
}

// enter opens the selected row: a folder opens or closes, a session becomes a chat.
func (m *Model) enter(now time.Time) {
	_, _, tree, ti := m.selection(now)
	if m.pane == workspace.PaneTree && len(tree) > 0 && tree[ti].Folder {
		m.expanded[tree[ti].Cwd] = !tree[ti].Expanded
		return
	}
	id := m.selectedID(now)
	e, ok := m.entry(now, id)
	if !ok {
		return
	}
	switch workspace.Decide(e, m.source(now)) {
	case workspace.Switch:
		for i, s := range m.sessions {
			if m.ownedID(s) == id {
				m.openOwned(i, now)
			}
		}
	case workspace.Claimed:
		m.say(now, "sesi ini sedang dipakai di terminal lain; hanya bisa dilihat")
	case workspace.Recent:
		if m.confirmID == id && now.Sub(m.confirmAt) <= confirmTTL {
			m.confirmID = ""
			m.resume(e, now)
			return
		}
		m.confirmID, m.confirmAt = id, now
		m.say(now, "sesi ini baru berubah beberapa detik lalu; tekan Enter lagi untuk resume")
	case workspace.Resume:
		m.resume(e, now)
	}
}

func (m *Model) resume(e index.Entry, now time.Time) {
	switch {
	case e.Cwd == "":
		m.say(now, "folder sesi ini tidak diketahui; tidak bisa di-resume")
		return
	case !m.statDir(e.Cwd):
		m.say(now, "folder tidak ada di sistem ini: %s", e.Cwd)
		return
	}
	if old := m.ownedAll()[e.ID]; old != nil { // the dead PTY of this very session
		for i, s := range m.sessions {
			if s == old {
				m.removeSession(i)
				break
			}
		}
	}
	m.newSession(SpawnReq{Dir: e.Cwd, ConfigDir: e.ConfigDir, ResumeID: e.ID}, now)
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

func (m *Model) startNew(dir, account string, now time.Time) {
	dir = filepath.Clean(expandHome(dir)) // the folder key must equal the cwd Claude records
	if !m.statDir(dir) {
		m.say(now, "folder tidak ada: %s", dir)
		return
	}
	m.prefs.Set(dir, true)
	if err := m.prefs.Save(); err != nil {
		m.say(now, "gagal menyimpan pilihan folder: %v", err)
	}
	m.newSession(SpawnReq{Dir: dir, ConfigDir: m.accounts[account]}, now)
}

// ---- overlays

// enterChat gives the chat to PTY s: the focus must name the session the panel shows, or typed
// bytes would go to another one.
func (m *Model) enterChat(s *session.Session) {
	for i, t := range m.sessions {
		if t == s {
			m.focus, m.mode = i, input.Chat
			return
		}
	}
}

// leaveChat returns to the lists; a zoomed chat would hide them, so zoom ends too.
func (m *Model) leaveChat() {
	m.zoom = false
	m.mode = input.Work
	if m.inOffice {
		m.mode = input.Home
	}
}

func (m *Model) openOverlay(k overlayKind) {
	if m.mode != input.Overlay {
		m.prevMode = m.mode
	}
	m.mode, m.overlay = input.Overlay, k
}

func (m *Model) closeOverlay(now time.Time) {
	m.overlay = ovNone
	m.mode = m.prevMode
	if m.mode == input.Chat {
		if s := m.shown(now); s == nil || s.Exited() {
			m.leaveChat()
		}
	}
}

func (m *Model) openFilter() {
	m.filter = workspace.NewFilterPicker(index.Folders(m.entries), m.prefs)
	m.openOverlay(ovFilter)
}

func (m *Model) openNew() {
	folders := index.Folders(m.entries)
	if m.defaultDir != "" {
		known := false
		for _, f := range folders {
			known = known || f.Cwd == m.defaultDir
		}
		if !known {
			folders = append([]index.Folder{{Cwd: m.defaultDir}}, folders...)
		}
	}
	m.newPick = workspace.NewNewPicker(folders, m.accountNames, m.accountOf)
	m.openOverlay(ovNew)
}

// accountOf is the account with the most sessions in a folder: where a new session there probably belongs.
func (m *Model) accountOf(cwd string) string {
	n := map[string]int{}
	best := ""
	for _, e := range m.entries {
		if e.Cwd == cwd {
			if n[e.Account]++; n[e.Account] > n[best] {
				best = e.Account
			}
		}
	}
	return best
}

func (m *Model) toggleHelp(now time.Time) {
	switch {
	case m.mode != input.Overlay:
		m.helpScroll = 0
		m.openOverlay(ovHelp)
	case m.overlay == ovHelp:
		m.closeOverlay(now)
	}
}

// ---- events

// Input feeds one chunk read from the terminal. Bytes for the open session are written in one
// piece per run, in order with any command in the same chunk. Pasted text (between bracketed-paste
// markers) never acts as a command: in a chat it goes to Claude, in a text box it is typed, else dropped.
// The markers are passed on only if Claude turned bracketed paste on.
func (m *Model) Input(data []byte, now time.Time) {
	var batch []byte
	target := func() *session.Session {
		if m.mode == input.Chat && m.focus >= 0 && m.focus < len(m.sessions) && !m.sessions[m.focus].Exited() {
			return m.sessions[m.focus]
		}
		return nil
	}
	flush := func() {
		if s := target(); s != nil && len(batch) > 0 {
			s.Write(batch)
		}
		batch = nil
	}
	marker := func(mk []byte, paste bool) {
		m.inPaste = paste
		if s := target(); s != nil && s.BracketedPaste() {
			batch = append(batch, mk...)
		}
	}
	for i := 0; i < len(data); {
		switch {
		case bytes.HasPrefix(data[i:], pasteStart):
			marker(pasteStart, true)
			i += len(pasteStart)
			continue
		case bytes.HasPrefix(data[i:], pasteEnd):
			marker(pasteEnd, false)
			i += len(pasteEnd)
			continue
		}
		if m.inPaste {
			switch {
			case m.mode == input.Chat:
				batch = append(batch, data[i])
				i++
			case m.querying || (m.mode == input.Overlay && m.overlay == ovNew): // a text box: typed, never a command
				r, n := utf8.DecodeRune(data[i:])
				if r >= ' ' && r != utf8.RuneError && r != 0x7f {
					m.HandleEvent(input.Event{Kind: input.EvKey, Key: string(r)}, now)
				}
				i += n
			default:
				i++
			}
			continue
		}
		ev, n := input.DecodeOne(data[i:], m.toggle)
		i += n
		if m.mode == input.Chat && ev.Kind == input.EvKey && ev.Key != "toggle" && ev.Key != "f1" {
			batch = append(batch, ev.Raw...) // Claude's key, unchanged
			m.lastTyped = now
			continue
		}
		flush()
		m.HandleEvent(ev, now)
	}
	flush()
	m.syncSizes(now)
}

// HandleEvent applies one key or mouse event.
func (m *Model) HandleEvent(ev input.Event, now time.Time) {
	if ev.Kind == input.EvCellSize {
		m.SetCellSize(ev.X, ev.Y)
		return
	}
	if ev.Kind == input.EvKey && ev.Key == "f1" {
		m.toggleHelp(now)
		return
	}
	switch m.mode {
	case input.Chat:
		m.chatEvent(ev, now)
	case input.Work:
		m.workEvent(ev, now)
	case input.Home:
		m.homeEvent(ev, now)
	case input.Overlay:
		m.overlayEvent(ev, now)
	}
	m.syncSizes(now)
}

func (m *Model) chatEvent(ev input.Event, now time.Time) {
	if ev.Kind == input.EvKey {
		if ev.Key == "toggle" {
			m.leaveChat()
		} else if s := m.shown(now); s != nil && !s.Exited() {
			s.Write(ev.Raw)
		}
		return
	}
	if m.inOffice && ev.Press && ev.Btn == 0 && m.actorAt(ev.X, ev.Y) != "" {
		m.leaveChat()
		m.homeEvent(ev, now)
		return
	}
	r := workspace.Compute(m.w, m.h, len(m.activeRows(now)), m.zoom, true)
	if l, ok := m.officeLayout(m.w, m.h); ok && m.inOffice {
		r.Chat = l.chat
	}
	if r.Chat.Contains(ev.X, ev.Y) {
		if s := m.shown(now); s != nil && !s.Exited() && s.MouseWanted() {
			final := 'M'
			if !ev.Press {
				final = 'm'
			}
			s.Write([]byte(fmt.Sprintf("\x1b[<%d;%d;%d%c", ev.Btn, ev.X-r.Chat.X+1, ev.Y-r.Chat.Y+1, final)))
		}
		return
	}
	if ev.Press && ev.Btn == 0 { // a click on a list leaves the chat
		m.leaveChat()
		if m.mode == input.Work {
			m.workMouse(ev, now)
		}
	}
}

func (m *Model) workEvent(ev input.Event, now time.Time) {
	if ev.Kind == input.EvMouse {
		m.workMouse(ev, now)
		return
	}
	if m.querying {
		switch k := ev.Key; k {
		case "enter":
			m.querying = false
		case "esc":
			m.query, m.querying = "", false
		case "backspace":
			if r := []rune(m.query); len(r) > 0 {
				m.query = string(r[:len(r)-1])
			}
		default:
			if len([]rune(k)) == 1 && k >= " " {
				m.query += k
			}
		}
		return
	}
	switch ev.Key {
	case "toggle":
		if s := m.shown(now); s != nil && !s.Exited() {
			m.enterChat(s)
		} else {
			m.say(now, "tidak ada chat terbuka: pilih sesi lalu Enter")
		}
	case "tab":
		if m.pane == workspace.PaneActive {
			m.pane = workspace.PaneTree
		} else {
			m.pane = workspace.PaneActive
		}
	case "up", "k":
		m.move(-1, now)
	case "down", "j":
		m.move(1, now)
	case "enter":
		m.enter(now)
	case "esc":
		m.query = ""
	case "/":
		m.querying = true
	case "z":
		if s := m.shown(now); s != nil && !s.Exited() {
			m.zoom = true
			m.enterChat(s)
		} else {
			m.say(now, "buka sesi dulu (Enter), baru perbesar")
		}
	case "a":
		m.cycleAccount(now)
	case "f":
		m.openFilter()
	case "n":
		m.openNew()
	case "d":
		if id := m.selectedID(now); id != "" {
			m.detailID = id
			m.openOverlay(ovDetail)
		} else {
			m.say(now, "pilih sesi dulu")
		}
	case "v":
		switch {
		case m.officeActive():
			m.mode = input.Home
			_, m.inOffice = m.officeLayout(m.w, m.h)
		case m.office != nil && m.office.Reason != "":
			m.say(now, "kantor tidak aktif: %s", m.office.Reason)
		default:
			m.say(now, "kantor tidak aktif: tidak ada tema (--theme)")
		}
	case "x":
		id := m.selectedID(now)
		s := m.ownedAll()[id]
		switch {
		case s == nil:
			m.say(now, "hanya sesi yang dibuka nakama yang bisa ditutup di sini")
		case !s.Exited():
			m.say(now, "sesi masih berjalan; akhiri dari dalam sesi")
		default:
			for i, t := range m.sessions {
				if t == s {
					m.removeSession(i)
					break
				}
			}
		}
	case "m":
		m.mouseOn = !m.mouseOn
		if m.mouseOn {
			m.say(now, "mouse aktif")
		} else {
			m.say(now, "mouse mati: drag biasa menyalin teks")
		}
	case "q":
		m.quit(now)
	case "?", "h":
		m.toggleHelp(now)
	}
}

// workMouse handles a click or the wheel over the panels.
func (m *Model) workMouse(ev input.Event, now time.Time) {
	if !ev.Press {
		return
	}
	active, ai, tree, ti := m.selection(now)
	r := workspace.Compute(m.w, m.h, len(active), m.zoom, m.mode == input.Chat)
	pane := workspace.PaneChat
	switch {
	case r.Active.Contains(ev.X, ev.Y):
		pane = workspace.PaneActive
	case r.Tree.Contains(ev.X, ev.Y):
		pane = workspace.PaneTree
	}
	switch ev.Btn {
	case 64, 65:
		if pane != workspace.PaneChat {
			m.pane = pane
			m.move(map[int]int{64: -1, 65: 1}[ev.Btn], now)
		}
		return
	case 0:
	default:
		return
	}
	double := now.Sub(m.lastClick) <= dblClick && ev.X == m.lastX && ev.Y == m.lastY
	m.lastClick, m.lastX, m.lastY = now, ev.X, ev.Y
	switch pane {
	case workspace.PaneActive:
		m.pane = pane
		if i := workspace.ScrollOffset(ai, r.Active.H-2) + ev.Y - r.Active.Y - 1; ev.Y > r.Active.Y && ev.Y < r.Active.Y+r.Active.H-1 && i >= 0 && i < len(active) {
			m.activeKey = active[i].ID
			if double {
				m.enter(now)
			}
		}
	case workspace.PaneTree:
		m.pane = pane
		if i := workspace.ScrollOffset(ti, r.Tree.H-2) + ev.Y - r.Tree.Y - 1; ev.Y > r.Tree.Y && ev.Y < r.Tree.Y+r.Tree.H-1 && i >= 0 && i < len(tree) {
			m.treeKey = rowKey(tree[i])
			if double {
				m.enter(now)
			}
		}
	default:
		if s := m.shown(now); s != nil && !s.Exited() && r.Chat.Contains(ev.X, ev.Y) {
			m.enterChat(s)
		}
	}
}

func (m *Model) homeEvent(ev input.Event, now time.Time) {
	if ev.Kind != input.EvKey {
		if ev.Press && ev.Btn == 0 {
			if id := m.actorAt(ev.X, ev.Y); id != "" {
				m.selected = id
				m.talk(now)
				return
			}
		}
		if l, ok := m.officeLayout(m.w, m.h); ok && ev.Press && ev.Btn == 0 && l.chat.Contains(ev.X, ev.Y) {
			m.talk(now) // a click on the chat panel is Enter
		}
		return
	}
	switch ev.Key {
	case "tab", "right", "down":
		m.selectStep(1)
	case "shift-tab", "left", "up":
		m.selectStep(-1)
	case "i":
		m.cardOpen = !m.cardOpen
	case "enter":
		m.talk(now)
	case "toggle":
		if s := m.ownedAll()[m.owner[m.selected]]; s != nil && !s.Exited() {
			m.talk(now)
		}
	case "z":
		if s := m.ownedAll()[m.owner[m.selected]]; s != nil && !s.Exited() {
			m.inOffice, m.zoom = false, true
			m.enterChat(s)
		} else {
			m.say(now, "buka sesi dulu (Enter), baru perbesar")
		}
	case "v", "esc":
		m.mode, m.inOffice = input.Work, false
	case "q":
		m.quit(now)
	}
}

// talk opens the selected agent's session: in the office's chat panel when there is room for one,
// else full screen. The rules are those of Enter on its row in Aktif.
func (m *Model) talk(now time.Time) {
	id, ok := m.owner[m.selected]
	if !ok {
		return
	}
	_, m.inOffice = m.officeLayout(m.w, m.h)
	m.pane, m.activeKey = workspace.PaneActive, id
	m.enter(now)
}

func (m *Model) overlayEvent(ev input.Event, now time.Time) {
	if ev.Kind == input.EvMouse {
		if m.overlay == ovHelp && ev.Press {
			m.helpScroll = max(m.helpScroll+map[int]int{64: -3, 65: 3}[ev.Btn], 0)
		}
		return
	}
	k := ev.Key
	switch m.overlay {
	case ovHelp:
		switch k {
		case "esc", "q", "?", "h":
			m.closeOverlay(now)
		case "up", "k":
			m.helpScroll = max(m.helpScroll-1, 0)
		case "down", "j":
			m.helpScroll++
		case "pgup":
			m.helpScroll = max(m.helpScroll-10, 0)
		case "pgdn":
			m.helpScroll += 10
		}
	case ovFilter:
		if m.filter.Key(k) {
			if m.filter.Saved() {
				m.filter.Apply(m.prefs)
				if err := m.prefs.Save(); err != nil {
					m.say(now, "gagal menyimpan pilihan folder: %v", err)
				}
			}
			m.closeOverlay(now)
		}
	case ovNew:
		if m.newPick.Key(k) {
			dir, acct, ok := m.newPick.Result()
			m.closeOverlay(now)
			if ok {
				m.startNew(dir, acct, now)
			}
		}
	case ovDetail:
		if k == "esc" || k == "q" || k == "d" {
			m.closeOverlay(now)
		}
	}
}
