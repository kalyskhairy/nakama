package transcript

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	asstText   = `{"type":"assistant","timestamp":"2026-11-25T10:00:00Z","isSidechain":false,"message":{"id":"m1","model":"claude-sonnet-5-5","stop_reason":"end_turn","usage":{"input_tokens":2,"cache_creation_input_tokens":6535,"cache_read_input_tokens":51415,"output_tokens":1266},"content":[{"type":"text","text":"hi"}]}}`
	asstTools  = `{"type":"assistant","timestamp":"2026-11-25T10:00:05Z","message":{"id":"m2","model":"claude-sonnet-5-5","stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":9},"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{}},{"type":"tool_use","id":"t2","name":"Read","input":{}}]}}`
	userResult = `{"type":"user","timestamp":"2026-11-25T10:00:06Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`
	userPrompt = `{"type":"user","timestamp":"2026-11-25T10:00:07Z","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`
	userString = `{"type":"user","timestamp":"2026-11-25T10:00:08Z","message":{"role":"user","content":"hello"}}`
	permMode   = `{"type":"permission-mode","permissionMode":"auto","sessionId":"x"}`
)

func TestParseAssistantLine(t *testing.T) {
	ev := ParseLine([]byte(asstText))
	if len(ev) != 1 {
		t.Fatalf("got %d events", len(ev))
	}
	e := ev[0]
	if e.Kind != Assistant || e.MessageID != "m1" || e.Model != "claude-sonnet-5-5" || e.StopReason != "end_turn" {
		t.Fatalf("%+v", e)
	}
	if e.Usage != (Usage{Input: 2, CacheCreate: 6535, CacheRead: 51415, Output: 1266}) {
		t.Fatalf("usage %+v", e.Usage)
	}
	if !e.Time.Equal(time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("time %v", e.Time)
	}
}

func TestParseAssistantWithToolUse(t *testing.T) {
	ev := ParseLine([]byte(asstTools))
	if len(ev) != 3 || ev[0].Kind != Assistant || ev[0].StopReason != "tool_use" {
		t.Fatalf("%+v", ev)
	}
	if ev[1].Kind != ToolUse || ev[1].ToolID != "t1" || ev[1].ToolName != "Bash" ||
		ev[2].Kind != ToolUse || ev[2].ToolID != "t2" || ev[2].ToolName != "Read" {
		t.Fatalf("%+v", ev)
	}
}

func TestParseToolResult(t *testing.T) {
	ev := ParseLine([]byte(userResult))
	if len(ev) != 1 || ev[0].Kind != ToolResult || ev[0].ToolID != "t1" {
		t.Fatalf("%+v", ev)
	}
}

func TestParseUserPrompt(t *testing.T) {
	for _, line := range []string{userPrompt, userString} {
		ev := ParseLine([]byte(line))
		if len(ev) != 1 || ev[0].Kind != UserPrompt {
			t.Fatalf("%s -> %+v", line, ev)
		}
	}
}

func TestParsePermissionMode(t *testing.T) {
	ev := ParseLine([]byte(permMode))
	if len(ev) != 1 || ev[0].Kind != PermissionMode || ev[0].Mode != "auto" {
		t.Fatalf("%+v", ev)
	}
}

func TestParseIgnoresSidechainAndGarbage(t *testing.T) {
	for _, line := range []string{
		`{"type":"assistant","isSidechain":true,"message":{"id":"s","content":[]}}`,
		`{not json`, ``, `   `, `{"type":"attachment"}`, `{"type":"assistant"}`, `[]`, `null`,
	} {
		if ev := ParseLine([]byte(line)); len(ev) != 0 {
			t.Errorf("%q -> %+v", line, ev)
		}
	}
}

func write(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString(s)
}

func TestReaderReadsOnlyCompleteLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	r := NewReader(path)
	write(t, path, asstText+"\n"+userPrompt+"\n"+userResult[:30])
	ev, err := r.Poll()
	if err != nil || len(ev) != 2 {
		t.Fatalf("first poll: %v %+v", err, ev)
	}
	write(t, path, userResult[30:]+"\n")
	ev, err = r.Poll()
	if err != nil || len(ev) != 1 || ev[0].Kind != ToolResult {
		t.Fatalf("second poll: %v %+v", err, ev)
	}
	if ev, _ := r.Poll(); len(ev) != 0 {
		t.Fatalf("nothing new, got %+v", ev)
	}
}

func TestReaderMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "later.jsonl")
	r := NewReader(path)
	if ev, err := r.Poll(); ev != nil || err != nil {
		t.Fatalf("missing: %v %v", ev, err)
	}
	write(t, path, permMode+"\n")
	if ev, _ := r.Poll(); len(ev) != 1 {
		t.Fatalf("after create: %+v", ev)
	}
}

func TestReaderRestartsWhenFileShrinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	r := NewReader(path)
	write(t, path, asstText+"\n"+asstTools+"\n")
	r.Poll()
	os.WriteFile(path, []byte(permMode+"\n"), 0o644) // shorter than the old offset
	ev, err := r.Poll()
	if err != nil || len(ev) != 1 || ev[0].Kind != PermissionMode {
		t.Fatalf("after shrink: %v %+v", err, ev)
	}
}

const sideAsst = `{"type":"assistant","isSidechain":true,"timestamp":"2026-11-25T10:00:00Z","message":{"id":"s1","model":"claude-haiku-5-5","stop_reason":"tool_use","usage":{"input_tokens":5,"output_tokens":7},"content":[{"type":"tool_use","id":"t9","name":"Bash","input":{}}]}}`

func TestSidechainReaderAcceptsSidechainLines(t *testing.T) {
	if ev := ParseLine([]byte(sideAsst)); len(ev) != 0 {
		t.Fatalf("ParseLine must still drop sidechain lines, got %+v", ev)
	}
	ev := ParseLineSidechain([]byte(sideAsst))
	if len(ev) != 2 || ev[0].Kind != Assistant || ev[1].Kind != ToolUse || ev[1].ToolName != "Bash" {
		t.Fatalf("ParseLineSidechain: %+v", ev)
	}
	path := filepath.Join(t.TempDir(), "agent.jsonl")
	write(t, path, sideAsst+"\n")
	if ev, _ := NewReader(path).Poll(); len(ev) != 0 {
		t.Fatalf("plain reader returned %+v", ev)
	}
	if ev, _ := NewSidechainReader(path).Poll(); len(ev) != 2 {
		t.Fatalf("sidechain reader returned %+v", ev)
	}
}

func tailFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTailReaderSkipsOldContent(t *testing.T) {
	old := asstText + "\n" + asstText + "\n" + asstText + "\n"
	p := tailFile(t, old+asstTools+"\n")
	ev, err := NewTailReader(p, int64(len(asstTools))+10).Poll() // cuts the last old line in half
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, e := range ev {
		if e.Kind == Assistant {
			ids = append(ids, e.MessageID)
		}
	}
	if len(ids) != 1 || ids[0] != "m2" {
		t.Fatalf("got %v", ids)
	}
}

func TestTailReaderThenFollows(t *testing.T) {
	p := tailFile(t, asstText+"\n")
	r := NewTailReader(p, 1<<20)
	if ev, _ := r.Poll(); len(ev) != 1 {
		t.Fatalf("first poll %d", len(ev))
	}
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(asstTools + "\n")
	f.Close()
	ev, _ := r.Poll()
	if len(ev) == 0 || ev[0].MessageID != "m2" {
		t.Fatalf("follow-up poll: %+v", ev)
	}
}

func TestSendMessageKeepsItsReceiver(t *testing.T) {
	line := `{"type":"assistant","message":{"id":"m","content":[{"type":"tool_use","id":"t","name":"SendMessage","input":{"to":"peer-1","message":"rahasia"}}]}}`
	evs := ParseLine([]byte(line))
	if len(evs) != 2 || evs[1].To != "peer-1" {
		t.Fatalf("%+v", evs)
	}
}
