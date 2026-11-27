package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConversationKeepsWhatWasSaid(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	lines := []string{
		`{"type":"user","message":{"content":"fix the coupon rounding"}}`,
		`{"type":"user","isMeta":true,"message":{"content":"meta note"}}`,
		`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"secret"},{"type":"text","text":"Rounding is in cart.go."},{"type":"tool_use","name":"Read"}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","content":"file body"}]}}`,
		`{"type":"user","message":{"content":[{"type":"text","text":"<system-reminder>injected</system-reminder>"},{"type":"text","text":"now add a test"}]}}`,
		`{"type":"assistant","isSidechain":true,"message":{"content":[{"type":"text","text":"subagent talk"}]}}`,
		`{"type":"summary","summary":"x"}`,
		`not json`,
	}
	os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
	got, err := Conversation(p)
	if err != nil {
		t.Fatal(err)
	}
	want := "## User\n\nfix the coupon rounding\n\n## Claude\n\nRounding is in cart.go.\n\n## User\n\nnow add a test\n"
	if got != want {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
}

func TestConversationKeepsTheEndOfALongSession(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	var b strings.Builder
	for i := 0; i < 3000; i++ {
		b.WriteString(`{"type":"user","message":{"content":"message number ` + strings.Repeat("x", 40) + `"}}` + "\n")
	}
	b.WriteString(`{"type":"user","message":{"content":"the very last one"}}` + "\n")
	os.WriteFile(p, []byte(b.String()), 0o600)
	got, _ := Conversation(p)
	if len(got) > handoffKeep+200 || !strings.HasSuffix(got, "the very last one\n") || !strings.HasPrefix(got, "(the earlier part") {
		t.Fatalf("%d bytes, ends %q", len(got), got[max(len(got)-40, 0):])
	}
}
