package workspace

import (
	"sort"
	"strings"
	"time"

	"github.com/kalyskhairy/nakama/internal/index"
	"github.com/kalyskhairy/nakama/internal/live"
	"github.com/kalyskhairy/nakama/internal/prefs"
	"github.com/kalyskhairy/nakama/internal/ui"
)

const (
	ActiveWindow = 5 * time.Minute  // a session that changed this recently counts as active
	RecentWindow = 30 * time.Second // changed this recently without a known process: maybe in use elsewhere
)

// Source is everything the rows are built from.
type Source struct {
	Entries     []index.Entry            // newest first
	Live        map[string]live.Proc     // running claude processes by session id
	Owned       map[string]bool          // session ids with a PTY of nakama's own
	State       func(id string) ui.State // from the monitor; nil means idle
	Now         time.Time
	TreeAccount string // show only this account in the tree; "" means all. Aktif always shows all.
}

func (s Source) state(id string) ui.State {
	if s.State == nil {
		return ui.StateIdle
	}
	return s.State(id)
}

type ActiveRow struct {
	ID, Folder, Title string
	Account           string
	State             ui.State
	Owned, Claimed    bool // Claimed: running in a terminal that is not nakama's
}

func rank(st ui.State) int {
	switch st {
	case ui.StateWaiting:
		return 0
	case ui.StateWorking:
		return 1
	case ui.StateActive:
		return 2
	}
	return 3
}

// BuildActive lists the sessions that matter right now: waiting first, then working, active, idle.
func BuildActive(s Source) []ActiveRow {
	var rows []ActiveRow
	mod := map[string]time.Time{}
	for _, e := range s.Entries {
		_, running := s.Live[e.ID]
		if !running && !s.Owned[e.ID] && s.Now.Sub(e.ModTime) > ActiveWindow {
			continue
		}
		rows = append(rows, ActiveRow{ID: e.ID, Account: e.Account, Folder: index.FolderName(e.Cwd), Title: e.Title, State: s.state(e.ID),
			Owned: s.Owned[e.ID], Claimed: running && !s.Owned[e.ID]})
		mod[e.ID] = e.ModTime
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if a, b := rank(rows[i].State), rank(rows[j].State); a != b {
			return a < b
		}
		return mod[rows[i].ID].After(mod[rows[j].ID])
	})
	return rows
}

type TreeRow struct {
	Folder         bool
	Cwd, ID, Title string
	Account        string
	State          ui.State
	Owned, Claimed bool
	Count          int
	Expanded       bool
}

func has(s, q string) bool { return strings.Contains(strings.ToLower(s), q) }

// BuildTree lists the shown folders, newest first, and the sessions of the expanded ones. A query
// keeps what matches and opens every folder it keeps.
func BuildTree(s Source, f *prefs.Folders, expanded map[string]bool, query string) []TreeRow {
	q := strings.ToLower(strings.TrimSpace(query))
	var rows []TreeRow
	for _, fo := range index.Folders(s.Entries) {
		if !f.Shown(fo.Cwd) {
			continue
		}
		name := index.FolderName(fo.Cwd)
		nameHit := q == "" || has(name, q)
		var sess []TreeRow
		count := 0
		for _, e := range s.Entries {
			if e.Cwd != fo.Cwd || (s.TreeAccount != "" && e.Account != s.TreeAccount) {
				continue
			}
			count++
			if !(nameHit || has(e.Title, q)) {
				continue
			}
			_, running := s.Live[e.ID]
			sess = append(sess, TreeRow{Cwd: e.Cwd, ID: e.ID, Title: e.Title, Account: e.Account, State: s.state(e.ID),
				Owned: s.Owned[e.ID], Claimed: running && !s.Owned[e.ID]})
		}
		if len(sess) == 0 {
			continue
		}
		open := expanded[fo.Cwd] || q != ""
		rows = append(rows, TreeRow{Folder: true, Cwd: fo.Cwd, Title: name, Count: count, Expanded: open})
		if open {
			rows = append(rows, sess...)
		}
	}
	return rows
}

type Verdict int

const (
	Switch  Verdict = iota // nakama already runs it: show it
	Resume                 // nobody runs it: resume it in a PTY
	Claimed                // another terminal runs it: look only
	Recent                 // changed seconds ago, no process known: ask first
)

// Decide says what Enter does on a session. Two processes must never write one transcript.
func Decide(e index.Entry, s Source) Verdict {
	switch {
	case s.Owned[e.ID]:
		return Switch
	case func() bool { _, ok := s.Live[e.ID]; return ok }():
		return Claimed
	case s.Now.Sub(e.ModTime) <= RecentWindow:
		return Recent
	}
	return Resume
}
