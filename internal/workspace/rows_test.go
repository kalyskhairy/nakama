package workspace

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kalyskhairy/nakama/internal/index"
	"github.com/kalyskhairy/nakama/internal/live"
	"github.com/kalyskhairy/nakama/internal/prefs"
	"github.com/kalyskhairy/nakama/internal/ui"
)

var now = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

func ent(id, cwd, title string, age time.Duration) index.Entry {
	return index.Entry{ID: id, Cwd: cwd, Title: title, Account: "personal", ModTime: now.Add(-age)}
}

func src(es ...index.Entry) Source {
	return Source{Entries: es, Live: map[string]live.Proc{}, Owned: map[string]bool{}, Now: now,
		State: func(string) ui.State { return ui.StateIdle }}
}

func ids(rows []ActiveRow) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

func shown(t *testing.T, cwds ...string) *prefs.Folders {
	f := prefs.Load(filepath.Join(t.TempDir(), "f.json"))
	for _, c := range cwds {
		f.Set(c, true)
	}
	return f
}

func TestActiveIncludesLiveOwnedAndRecent(t *testing.T) {
	s := src(ent("live", "/a", "L", time.Hour), ent("owned", "/a", "O", 2*time.Hour), ent("recent", "/a", "R", time.Minute), ent("old", "/a", "X", time.Hour))
	s.Live["live"] = live.Proc{PID: 1, SessionID: "live"}
	s.Owned["owned"] = true
	rows := BuildActive(s)
	got := map[string]ActiveRow{}
	for _, r := range rows {
		got[r.ID] = r
	}
	if len(rows) != 3 || got["old"].ID != "" {
		t.Fatalf("%v", ids(rows))
	}
	if !got["live"].Claimed || got["live"].Owned || !got["owned"].Owned || got["owned"].Claimed || got["recent"].Claimed {
		t.Fatalf("%+v", got)
	}
	if got["live"].Folder != "a" || got["live"].Title != "L" {
		t.Fatalf("%+v", got["live"])
	}
}

func TestActiveOrdersWaitingFirst(t *testing.T) {
	s := src(ent("idle", "/a", "", time.Minute), ent("work", "/a", "", 2*time.Minute), ent("wait", "/a", "", 3*time.Minute), ent("wait2", "/a", "", 4*time.Minute))
	s.State = func(id string) ui.State {
		return map[string]ui.State{"wait": ui.StateWaiting, "wait2": ui.StateWaiting, "work": ui.StateWorking}[id]
	}
	if got := ids(BuildActive(s)); len(got) != 4 || got[0] != "wait" || got[1] != "wait2" || got[2] != "work" || got[3] != "idle" {
		t.Fatalf("%v", got)
	}
}

func TestWaitingSessionInHiddenFolderStillActive(t *testing.T) {
	s := src(ent("w", "/hidden", "T", time.Minute))
	s.State = func(string) ui.State { return ui.StateWaiting }
	if got := BuildActive(s); len(got) != 1 || got[0].State != ui.StateWaiting {
		t.Fatalf("%+v", got)
	}
	if rows := BuildTree(s, shown(t, "/other"), nil, ""); len(rows) != 0 {
		t.Fatalf("hidden folder leaked into the tree: %+v", rows)
	}
}

func TestTreeHidesUnshownFoldersAndExpands(t *testing.T) {
	s := src(ent("1", "/a", "one", time.Hour), ent("2", "/a", "two", 2*time.Hour), ent("3", "/b", "three", time.Minute), ent("4", "/c", "four", time.Second))
	f := shown(t, "/a", "/b")
	rows := BuildTree(s, f, map[string]bool{"/a": true}, "")
	var got []string
	for _, r := range rows {
		got = append(got, r.Title)
	}
	want := []string{"b", "a", "one", "two"} // /b is newer than /a; /a is expanded; /c is hidden
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%v want %v", got, want)
		}
	}
	if !rows[0].Folder || rows[0].Count != 1 || rows[0].Expanded || !rows[1].Expanded || rows[2].Folder {
		t.Fatalf("%+v", rows)
	}
}

func TestTreeQueryFilters(t *testing.T) {
	s := src(ent("1", "/a", "Referral endpoint", time.Hour), ent("2", "/a", "Lainnya", 2*time.Hour), ent("3", "/zeta", "x", time.Minute))
	f := shown(t, "/a", "/zeta")
	rows := BuildTree(s, f, nil, "referral")
	if len(rows) != 2 || rows[0].Title != "a" || rows[1].Title != "Referral endpoint" {
		t.Fatalf("%+v", rows)
	}
	rows = BuildTree(s, f, nil, "ZETA") // folder name matches: its sessions show, any letter case
	if len(rows) != 2 || rows[0].Title != "zeta" || rows[1].ID != "3" {
		t.Fatalf("%+v", rows)
	}
}

func TestDecide(t *testing.T) {
	s := src(ent("owned", "/a", "", time.Hour), ent("claimed", "/a", "", time.Hour), ent("recent", "/a", "", 10*time.Second), ent("edge", "/a", "", 31*time.Second), ent("old", "/a", "", time.Hour))
	s.Owned["owned"] = true
	s.Live["claimed"] = live.Proc{SessionID: "claimed"}
	want := map[string]Verdict{"owned": Switch, "claimed": Claimed, "recent": Recent, "edge": Resume, "old": Resume}
	for _, e := range s.Entries {
		if got := Decide(e, s); got != want[e.ID] {
			t.Errorf("%s: %v want %v", e.ID, got, want[e.ID])
		}
	}
}

func TestTreeAccountFilterAndActiveCarriesAccount(t *testing.T) {
	w := ent("w", "/a", "kerja", time.Minute)
	w.Account = "work"
	s := src(w, ent("p", "/a", "pribadi", time.Hour), ent("only", "/b", "x", time.Hour))
	f := shown(t, "/a", "/b")
	s.TreeAccount = "work"
	rows := BuildTree(s, f, map[string]bool{"/a": true, "/b": true}, "")
	if len(rows) != 2 || rows[0].Count != 1 || rows[1].ID != "w" {
		t.Fatalf("only the work session and its folder, counted alone: %+v", rows)
	}
	s.TreeAccount = "personal"
	if rows := BuildTree(s, f, map[string]bool{"/a": true, "/b": true}, ""); len(rows) != 4 {
		t.Fatalf("%+v", rows)
	}
	if a := BuildActive(s); len(a) != 1 || a[0].ID != "w" || a[0].Account != "work" {
		t.Fatalf("Aktif ignores the account filter and says whose session it is: %+v", a)
	}
}
