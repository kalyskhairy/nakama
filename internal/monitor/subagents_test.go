package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	subTool     = `{"type":"assistant","isSidechain":true,"timestamp":"TS","message":{"id":"m-AID","model":"claude-haiku-5-5","stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":5},"content":[{"type":"tool_use","id":"t-AID","name":"Bash","input":{}}]}}`
	subText     = `{"type":"assistant","isSidechain":true,"timestamp":"TS","message":{"id":"m-AID","model":"claude-haiku-5-5","stop_reason":"end_turn","usage":{"input_tokens":100,"output_tokens":5},"content":[{"type":"text","text":"done"}]}}`
	subMid      = `{"type":"assistant","isSidechain":true,"timestamp":"TS","message":{"id":"m-AID","model":"claude-haiku-5-5","stop_reason":null,"usage":{"input_tokens":100,"output_tokens":5},"content":[{"type":"text","text":"..."}]}}`
	subHandback = `{"type":"assistant","isSidechain":true,"timestamp":"TS","message":{"id":"h-AID","stop_reason":"tool_use","content":[{"type":"tool_use","id":"hb-AID","name":"SubagentHandback","input":{}}]}}
{"type":"user","isSidechain":true,"timestamp":"TS","message":{"content":[{"type":"tool_result","tool_use_id":"hb-AID","content":"ok"}]}}`
)

// session lays out <root>/-x/<id>.jsonl with the parent in permission mode default.
func session(t *testing.T, root, id string) (fm *FileMonitor, dir string) {
	t.Helper()
	appendTo(t, filepath.Join(root, "-x", id+".jsonl"), lines(t0, lnMode))
	return NewFileMonitor(func() []string { return []string{root} }, 200000), filepath.Join(root, "-x", id, "subagents")
}

func agent(t *testing.T, dir, aid, line, meta string) {
	t.Helper()
	appendTo(t, filepath.Join(dir, "agent-"+aid+".jsonl"), lines(t0, strings.ReplaceAll(line, "AID", aid)))
	if meta != "" {
		os.WriteFile(filepath.Join(dir, "agent-"+aid+".meta.json"), []byte(meta), 0o644)
	}
}

func TestSubagentsDiscoveredWithTypeAndDescription(t *testing.T) {
	root := t.TempDir()
	fm, dir := session(t, root, "s1")
	agent(t, dir, "a", subMid, `{"agentType":"Explore","description":"cari x","finished":false}`)
	agent(t, dir, "b", subMid, `{"agentType":"Plan","description":"rencana y"}`)
	fm.Poll("s1", t0)
	subs := fm.Subagents("s1", t0.Add(time.Second))
	if len(subs) != 2 || subs[0].ID != "a" || subs[0].Type != "Explore" || subs[0].Description != "cari x" || subs[1].Type != "Plan" {
		t.Fatalf("%+v", subs)
	}
	fm.Poll("s1", t0.Add(time.Second)) // polling again must not duplicate
	if n := len(fm.Subagents("s1", t0.Add(time.Second))); n != 2 {
		t.Fatalf("%d after a second poll", n)
	}
}

func TestSubagentStatusFromItsOwnTranscript(t *testing.T) {
	root := t.TempDir()
	fm, dir := session(t, root, "s1")
	agent(t, dir, "a", subTool, `{"agentType":"general-purpose"}`)
	fm.Poll("s1", t0)
	if got := fm.Subagents("s1", t0.Add(2*time.Second))[0].Info.Status; got != Working {
		t.Fatalf("2 s: %v", got)
	}
	in := fm.Subagents("s1", t0.Add(4*time.Second))[0].Info
	if in.Status != Waiting || in.PendingTool != "Bash" { // permission mode default comes from the parent
		t.Fatalf("4 s: %+v", in)
	}
}

func TestFinishedSubagentsDisappear(t *testing.T) {
	root := t.TempDir()
	fm, dir := session(t, root, "s1")
	agent(t, dir, "flag", subTool, `{"agentType":"Explore","finished":true}`)
	agent(t, dir, "ended", subText, `{"agentType":"Explore"}`)      // finished never set, like many real ones
	agent(t, dir, "handed", subHandback, `{"agentType":"Explore"}`) // reported back to its parent
	agent(t, dir, "busy", subTool, `{"agentType":"Explore"}`)
	fm.Poll("s1", t0)
	ids := func() (out string) {
		for _, s := range fm.Subagents("s1", t0.Add(time.Second)) {
			out += s.ID + " "
		}
		return out
	}
	if got := ids(); got != "busy " {
		t.Fatalf("only the busy one must show: %q", got)
	}
	agent(t, dir, "ended", subTool, "") // sent a new message: it works again
	fm.Poll("s1", t0)
	if got := ids(); got != "busy ended " {
		t.Fatalf("a resumed subagent must come back: %q", got)
	}
}

func TestStaleSubagentsAreHidden(t *testing.T) {
	root := t.TempDir()
	fm, dir := session(t, root, "s1")
	agent(t, dir, "stale", subMid, `{"agentType":"Explore"}`)
	fm.Poll("s1", t0)
	if n := len(fm.Subagents("s1", t0.Add(time.Second))); n != 1 {
		t.Fatalf("%d", n)
	}
	if n := len(fm.Subagents("s1", t0.Add(400*time.Second))); n != 0 {
		t.Fatalf("quiet for 400 s: %d", n)
	}
}

func TestSubagentAppearingLaterIsFound(t *testing.T) {
	root := t.TempDir()
	fm, dir := session(t, root, "s1")
	fm.Poll("s1", t0)
	if n := len(fm.Subagents("s1", t0)); n != 0 {
		t.Fatalf("%d before any file", n)
	}
	agent(t, dir, "late", subMid, `{"agentType":"Explore"}`)
	fm.Poll("s1", t0.Add(time.Second))
	if n := len(fm.Subagents("s1", t0.Add(time.Second))); n != 1 {
		t.Fatalf("%d after the file appeared", n)
	}
}

func TestSubagentCapAndBrokenMeta(t *testing.T) {
	root := t.TempDir()
	fm, dir := session(t, root, "s1")
	for i := 0; i < 20; i++ {
		agent(t, dir, fmt.Sprintf("a%02d", i), subMid, `{"agentType":"Explore"}`)
	}
	fm.Poll("s1", t0)
	if n := len(fm.Subagents("s1", t0)); n != 12 {
		t.Fatalf("%d subagents, want the cap of 12", n)
	}

	root2 := t.TempDir()
	fm2, dir2 := session(t, root2, "s2")
	agent(t, dir2, "x", subMid, `{broken`)
	fm2.Poll("s2", t0)
	if got := fm2.Subagents("s2", t0); len(got) != 1 || got[0].Type != "" {
		t.Fatalf("broken meta: %+v", got)
	}
	if got := fm2.Subagents("unknown-session", t0); len(got) != 0 {
		t.Fatalf("unknown session: %+v", got)
	}
}

func TestRestingSubagentsLeave(t *testing.T) {
	root := t.TempDir()
	fm, dir := session(t, root, "s1")
	agent(t, dir, "think", subMid, `{"agentType":"Explore"}`)
	agent(t, dir, "tool", subTool, `{"agentType":"Plan"}`) // a tool that runs for a long time
	fm.Poll("s1", t0)
	at := func(d time.Duration) (out string) {
		for _, s := range fm.Subagents("s1", t0.Add(d)) {
			if s.Info.Status == Idle {
				t.Fatalf("at %v %s is shown resting", d, s.ID)
			}
			out += s.ID + " "
		}
		return out
	}
	if got := at(45 * time.Second); got != "think tool " {
		t.Fatalf("45 s: %q", got)
	}
	if got := at(90 * time.Second); got != "tool " {
		t.Fatalf("90 s: a quiet one must be gone: %q", got)
	}
	if got := at(10 * time.Minute); got != "tool " {
		t.Fatalf("10 min: a subagent with a tool still running is not resting: %q", got)
	}
}
