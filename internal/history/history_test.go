package history

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 3, 10, 15, 0, 0, 0, time.UTC)

func put(t *testing.T, root, rel string, lines ...string) {
	t.Helper()
	p := filepath.Join(root, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func asst(ts, id string, in, out int, text string) string {
	return `{"type":"assistant","timestamp":"` + ts + `","cwd":"/p/shop","message":{"id":"` + id + `","content":[{"type":"text","text":"` + text + `"}],"usage":{"input_tokens":` +
		strconv.Itoa(in) + `,"cache_creation_input_tokens":0,"cache_read_input_tokens":999,"output_tokens":` + strconv.Itoa(out) + `}}}`
}

func user(ts, text string) string {
	return `{"type":"user","timestamp":"` + ts + `","cwd":"/p/shop","message":{"content":"` + text + `"}}`
}

func TestUsageAddsUpByDayAndSessionOnceEach(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".claude", "projects")
	put(t, root, "-p-shop/s1.jsonl", asst("2026-03-10T09:00:00Z", "a", 10, 5, "x"), asst("2026-03-10T09:00:01Z", "a", 10, 5, "x"),
		asst("2026-03-09T09:00:00Z", "b", 100, 0, "x"), asst("2026-01-01T09:00:00Z", "old", 7000, 0, "x"))
	put(t, root, "-p-shop/s1/subagents/agent-1.jsonl", asst("2026-03-10T10:00:00Z", "c", 1, 1, "x"))
	put(t, root, "-p-shop/s2.jsonl", asst("2026-03-10T11:00:00Z", "d", 500, 0, "x"), "{broken")
	r := Usage([]string{root}, now.AddDate(0, 0, -6), time.UTC)
	if r.Days["2026-03-10"] != 517 || r.Days["2026-03-09"] != 100 || len(r.Days) != 2 {
		t.Fatalf("days %v", r.Days)
	}
	today := r.Sessions("2026-03-10")
	if len(today) != 2 || today[0].ID != "s2" || today[0].Tokens != 500 || today[1].ID != "s1" || today[1].Tokens != 17 {
		t.Fatalf("sessions %+v", today) // the subagent counts for its session; the largest first
	}
	if today[0].Account != "personal" || today[0].Cwd != "/p/shop" {
		t.Fatalf("%+v", today[0])
	}
}

func TestSearchFindsWhatWasSaidNotTheMachinery(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".claude-work", "projects")
	put(t, root, "-p-shop/old.jsonl", user("2026-03-01T09:00:00Z", "please fix the Coupon rounding"))
	put(t, root, "-p-shop/new.jsonl",
		`{"type":"user","timestamp":"2026-03-09T09:00:00Z","message":{"content":[{"type":"tool_result","content":"coupon in a file"}]}}`,
		asst("2026-03-09T09:00:00Z", "m", 1, 1, "The coupon logic is in cart.go"), user("2026-03-09T09:05:00Z", "coupon again"))
	put(t, root, "-p-shop/none.jsonl", user("2026-03-09T09:00:00Z", "nothing here"))
	put(t, root, "-p-shop/new/subagents/agent-1.jsonl", user("2026-03-09T09:00:00Z", "coupon subagent"))
	got := Search([]string{root}, "COUPON", 10)
	if len(got) != 2 || got[0].ID != "new" || got[1].ID != "old" {
		t.Fatalf("%+v", got)
	}
	if got[0].Count != 2 || !strings.Contains(got[0].Snippet, "coupon again") || got[0].Account != "work" || got[1].Cwd != "/p/shop" {
		t.Fatalf("%+v", got[0])
	}
	if len(Search([]string{root}, "  ", 10)) != 0 || len(Search([]string{root}, "coupon", 1)) != 1 {
		t.Fatal("blank query or limit")
	}
}
