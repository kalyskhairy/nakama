package app

import (
	"strings"
	"testing"
	"time"

	"github.com/kalyskhairy/nakama/internal/input"
	"github.com/kalyskhairy/nakama/internal/live"
	"github.com/kalyskhairy/nakama/internal/workspace"
)

func TestQuestionMarkClosesHelp(t *testing.T) {
	for _, k := range []string{"?", "h"} {
		m := newModel(t, spawnCmd("sleep", "30"))
		m.HandleEvent(key("?"), t0)
		m.HandleEvent(key(k), t0)
		if m.Mode() != input.Work {
			t.Errorf("%q: mode %v", k, m.Mode())
		}
	}
}

func TestClickOnABottomBorderSelectsNothing(t *testing.T) {
	var es = ent("s00", "/p", "J", time.Minute)
	m, _, _ := workModel(t, nil, es)
	for i := 1; i < 40; i++ {
		m.entries = append(m.entries, ent("s"+strings.Repeat("x", i), "/p", "J", time.Duration(i+1)*time.Minute))
	}
	r := workspace.Compute(m.w, m.h, len(m.activeRows(t0)), false, false)
	m.activeKey = "s00"
	m.HandleEvent(click(r.Active.X+2, r.Active.Y+r.Active.H-1), t0)
	if m.activeKey != "s00" {
		t.Fatalf("the border selected %q", m.activeKey)
	}
}

func TestToggleWithoutAChatSaysWhy(t *testing.T) {
	m := newModel(t, spawnCmd("sleep", "30"))
	m.HandleEvent(key("toggle"), t0)
	if m.Message(t0) == "" {
		t.Fatal("silent")
	}
}

func TestSelectionFollowsASessionWhoseIDChanges(t *testing.T) {
	m := newModel(t, spawnID("old", "sleep", "30"))
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	m.leaveChat()
	m.cast = map[string]string{"old": "fencer"}
	m.selected = "old"
	m.SetLive([]live.Proc{{PID: m.Sessions()[0].PID(), SessionID: "new"}})
	if m.activeKey != "new" || m.treeKey != "s:new" || m.selected != "new" || m.cast["new"] != "fencer" {
		t.Fatalf("active %q tree %q selected %q cast %v", m.activeKey, m.treeKey, m.selected, m.cast)
	}
}

func TestOurOwnExitedSessionIsNotCalledAnotherTerminal(t *testing.T) {
	m := newModel(t, spawnID("done", "true"))
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	waitFor(t, "exit", func() bool { return m.Sessions()[0].Exited() })
	m.SetLive([]live.Proc{{PID: m.Sessions()[0].PID(), SessionID: "done"}}) // read just before it ended
	for _, r := range m.activeRows(t0) {
		if r.Claimed {
			t.Fatalf("%+v", r)
		}
	}
}

func TestPasteReachesTheSearchBox(t *testing.T) {
	m := newModel(t, spawnCmd("sleep", "30"))
	m.Input([]byte("/"), t0)
	m.Input([]byte("\x1b[200~onboard q\x1b[201~"), t0)
	if m.query != "onboard q" || m.Quitting() {
		t.Fatalf("query %q", m.query)
	}
	m.Input([]byte("\r"), t0)
	m.Input([]byte("\x1b[200~q\x1b[201~"), t0)
	if m.Quitting() {
		t.Fatal("a paste outside a text box must never be a command")
	}
}
