package monitor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func lines(ts time.Time, parts ...string) string {
	s := ts.UTC().Format(time.RFC3339)
	return strings.NewReplacer("TS", s).Replace(strings.Join(parts, "\n") + "\n")
}

const (
	lnMode = `{"type":"permission-mode","permissionMode":"default"}`
	lnAsst = `{"type":"assistant","timestamp":"TS","message":{"id":"m1","model":"claude-sonnet-5-5","stop_reason":"tool_use","usage":{"input_tokens":30000,"cache_creation_input_tokens":0,"cache_read_input_tokens":10000,"output_tokens":100},"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{}}]}}`
)

func appendTo(t *testing.T, path, s string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString(s)
}

func TestFileMonitorEndToEnd(t *testing.T) {
	root := t.TempDir()
	fm := NewFileMonitor(func() []string { return []string{root} }, 200000)
	appendTo(t, filepath.Join(root, "-x", "id1.jsonl"), lines(t0, lnMode, lnAsst))
	fm.Poll("id1", t0)
	in := fm.Info("id1", t0.Add(4*time.Second))
	if in.Status != Waiting || in.PendingTool != "Bash" || in.CtxPct != 20 || in.TokensToday != 30100 {
		t.Fatalf("%+v", in)
	}
}

func TestFileMonitorHandlesMissingTranscript(t *testing.T) {
	root := t.TempDir()
	fm := NewFileMonitor(func() []string { return []string{root} }, 200000)
	fm.Poll("id1", t0)
	if in := fm.Info("id1", t0); in.Status != Idle || in.CtxPct != -1 {
		t.Fatalf("before file: %+v", in)
	}
	if in := fm.Info("never-polled", t0); in.CtxPct != -1 {
		t.Fatalf("unknown id: %+v", in)
	}
	appendTo(t, filepath.Join(root, "-x", "id1.jsonl"), lines(t0, lnAsst))
	fm.Poll("id1", t0.Add(time.Second))
	if in := fm.Info("id1", t0.Add(time.Second)); in.CtxPct != 20 {
		t.Fatalf("after file: %+v", in)
	}
}

func TestFileMonitorFindsRootThatAppearsLater(t *testing.T) {
	home := t.TempDir() // no .claude/projects yet, like a first run
	t.Setenv("HOME", home)
	fm := NewFileMonitor(Roots, 200000)
	fm.Poll("id1", t0)
	if in := fm.Info("id1", t0); in.CtxPct != -1 {
		t.Fatalf("nothing exists yet: %+v", in)
	}
	appendTo(t, filepath.Join(home, ".claude", "projects", "-x", "id1.jsonl"), lines(t0, lnAsst))
	fm.Poll("id1", t0.Add(time.Second))
	if in := fm.Info("id1", t0.Add(time.Second)); in.CtxPct != 20 {
		t.Fatalf("root created after start was not found: %+v", in)
	}
}

func TestFileMonitorTailsBigTranscript(t *testing.T) {
	root := t.TempDir()
	fm := NewFileMonitor(func() []string { return []string{root} }, 200000)
	fm.TailBytes = 4096
	path := filepath.Join(root, "-x", "big.jsonl")
	appendTo(t, path, `{"type":"permission-mode","permissionMode":"auto"}`+"\n") // only in the head: must not be read
	filler := `{"type":"summary","pad":"` + strings.Repeat("x", 200) + `"}` + "\n"
	appendTo(t, path, strings.Repeat(filler, 1000))
	appendTo(t, path, lines(t0, lnAsst)) // pending Bash; mode seen = default (none), so it asks
	fm.Poll("big", t0)
	if in := fm.Info("big", t0.Add(4*time.Second)); in.Status != Waiting {
		t.Fatalf("head was read (auto mode would not wait): %+v", in)
	}
}

func TestFileMonitorActivity(t *testing.T) {
	root := t.TempDir()
	fm := NewFileMonitor(func() []string { return []string{root} }, 200000)
	if fm.Activity("nope") != nil {
		t.Fatal("unknown id must give nil")
	}
	appendTo(t, filepath.Join(root, "-x", "a1.jsonl"), lines(t0, lnMode, lnAsst))
	fm.Poll("a1", t0)
	if got := fm.Activity("a1"); len(got) == 0 || got[0].Text != "alat: Bash" {
		t.Fatalf("%+v", got)
	}
}
