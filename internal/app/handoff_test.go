package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kalyskhairy/nakama/internal/input"
)

func TestCStartsANewSessionThatReadsTheOldOne(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.jsonl")
	os.WriteFile(path, []byte(`{"type":"user","message":{"content":"fix the coupon rounding"}}`+"\n"), 0o600)
	e := ent("old", "/proj", "Coupons", time.Hour)
	e.Path, e.Account, e.ConfigDir = path, "work", "/h/.claude-work"
	m, rs, _ := workModel(t, []string{"/proj"}, e)
	m.SetHandoffDir(filepath.Join(dir, "handoff"))
	selectSession(m, "old")
	m.HandleEvent(key("c"), t0)
	if len(rs.reqs) != 1 || m.Mode() != input.Chat {
		t.Fatalf("%+v mode %v", rs.reqs, m.Mode())
	}
	r := rs.reqs[0]
	if r.Dir != "/proj" || r.ConfigDir != "/h/.claude-work" || r.ResumeID != "" {
		t.Fatalf("same folder and account, a new session: %+v", r)
	}
	file := filepath.Join(dir, "handoff", "old.md")
	if !strings.Contains(r.Prompt, file) {
		t.Fatalf("prompt %q", r.Prompt)
	}
	b, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(b), "fix the coupon rounding") || !strings.Contains(string(b), "Coupons") {
		t.Fatalf("%v %q", err, b)
	}
	if st, _ := os.Stat(file); st.Mode().Perm() != 0o600 {
		t.Fatalf("the conversation stays private: %v", st.Mode())
	}
}

func TestCWithoutATranscriptSaysWhy(t *testing.T) {
	m, rs, _ := workModel(t, []string{"/proj"}, ent("old", "/proj", "Coupons", time.Hour))
	m.SetHandoffDir(t.TempDir())
	selectSession(m, "old")
	m.HandleEvent(key("c"), t0)
	if len(rs.reqs) != 0 || m.Message(t0) == "" {
		t.Fatalf("%+v %q", rs.reqs, m.Message(t0))
	}
}
