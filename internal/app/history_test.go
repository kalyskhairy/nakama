package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kalyskhairy/nakama/internal/input"
)

func historyRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), ".claude", "projects")
	os.MkdirAll(filepath.Join(root, "-p"), 0o755)
	ts := t0.Add(-time.Hour).UTC().Format(time.RFC3339)
	os.WriteFile(filepath.Join(root, "-p", "old.jsonl"), []byte(
		`{"type":"user","timestamp":"`+ts+`","cwd":"/p","message":{"content":"fix the coupon rounding"}}`+"\n"+
			`{"type":"assistant","timestamp":"`+ts+`","message":{"id":"m","model":"claude-opus-5-5","content":[],"usage":{"input_tokens":1500,"output_tokens":500}}}`+"\n"), 0o644)
	return root
}

func TestUsageScreen(t *testing.T) {
	m, _, _ := workModel(t, []string{"/p"}, ent("old", "/p", "Coupons", time.Hour))
	root := historyRoot(t)
	m.SetHistoryRoots(func() []string { return []string{root} })
	m.HandleEvent(key("u"), t0)
	f := ansiStrip(Frame(m, 140, 40, t0))
	for _, want := range []string{"Usage", "Last 7 days", "2.0K tok", "Today by session", "Coupons", "$0.02", "Last 7 days by account", "not your bill"} {
		if !strings.Contains(f, want) {
			t.Fatalf("no %q in %q", want, f)
		}
	}
	m.HandleEvent(key("u"), t0)
	if m.Mode() != input.Work {
		t.Fatalf("u closes it: %v", m.Mode())
	}
}

func TestSearchScreenFindsAndResumes(t *testing.T) {
	m, rs, _ := workModel(t, []string{"/p"}, ent("old", "/p", "Coupons", time.Hour))
	root := historyRoot(t)
	m.SetHistoryRoots(func() []string { return []string{root} })
	var queued []func() func()
	m.SetBackground(func(work func() func()) { queued = append(queued, work) })
	m.HandleEvent(key("s"), t0)
	m.Input([]byte("\x1b[200~coupon\x1b[201~"), t0)
	m.Input([]byte("\r"), t0)
	if f := ansiStrip(Frame(m, 140, 40, t0)); !strings.Contains(f, "Searching") || len(queued) != 1 {
		t.Fatalf("runs in the background: %d %q", len(queued), f)
	}
	queued[0]()()
	f := ansiStrip(Frame(m, 140, 40, t0))
	if !strings.Contains(f, "Coupons") || !strings.Contains(f, "fix the coupon rounding") {
		t.Fatalf("%q", f)
	}
	m.Input([]byte("\r"), t0)
	if len(rs.reqs) != 1 || rs.reqs[0].ResumeID != "old" || m.Mode() != input.Chat {
		t.Fatalf("Enter opens the hit: %+v mode %v", rs.reqs, m.Mode())
	}
}

func TestAnOutdatedSearchResultIsDropped(t *testing.T) {
	m, _, _ := workModel(t, nil)
	root := historyRoot(t)
	m.SetHistoryRoots(func() []string { return []string{root} })
	var queued []func() func()
	m.SetBackground(func(work func() func()) { queued = append(queued, work) })
	m.HandleEvent(key("s"), t0)
	for _, k := range []string{"c", "o", "enter", "x", "enter"} {
		m.HandleEvent(key(k), t0)
	}
	second := queued[1]()
	first := queued[0]()
	second()
	first()
	if len(m.hits) != 0 || m.searched != "cox" {
		t.Fatalf("%q %+v", m.searched, m.hits)
	}
}
