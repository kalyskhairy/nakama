package monitor

import (
	"fmt"
	"testing"
	"time"

	tr "github.com/kalyskhairy/nakama/internal/transcript"
)

var t0 = time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)

func use(id, name string) tr.Event {
	return tr.Event{Kind: tr.ToolUse, Time: t0, ToolID: id, ToolName: name}
}
func result(id string) tr.Event { return tr.Event{Kind: tr.ToolResult, Time: t0, ToolID: id} }
func mode(m string) tr.Event    { return tr.Event{Kind: tr.PermissionMode, Time: t0, Mode: m} }
func asst(id string, at time.Time, u tr.Usage) tr.Event {
	return tr.Event{Kind: tr.Assistant, Time: at, MessageID: id, Model: "claude-sonnet-5-5", Usage: u}
}

func tracker(evs ...tr.Event) *Tracker {
	k := NewTracker(200000)
	k.Apply(evs, t0)
	return k
}

func TestWaitingAfterThreeSecondsWhenToolAsks(t *testing.T) {
	k := tracker(mode("default"), use("t1", "Bash"))
	if got := k.Info(t0.Add(2 * time.Second)).Status; got != Working {
		t.Fatalf("2s: %v", got)
	}
	in := k.Info(t0.Add(4 * time.Second))
	if in.Status != Waiting || in.PendingTool != "Bash" {
		t.Fatalf("4s: %+v", in)
	}
}

func TestToolResultClearsPending(t *testing.T) {
	k := tracker(mode("default"), use("t1", "Bash"), result("t1"))
	if got := k.Info(t0.Add(4 * time.Second)).Status; got == Waiting || got == Working {
		t.Fatalf("got %v", got)
	}
}

func TestAutoModeNeverWaits(t *testing.T) {
	k := tracker(mode("auto"), use("t1", "Bash"))
	if got := k.Info(t0.Add(10 * time.Second)).Status; got != Working {
		t.Fatalf("got %v", got)
	}
}

func TestAskUserQuestionAlwaysWaits(t *testing.T) {
	k := tracker(mode("auto"), use("t1", "AskUserQuestion"))
	if got := k.Info(t0.Add(4 * time.Second)).Status; got != Waiting {
		t.Fatalf("got %v", got)
	}
}

func TestReadOnlyToolsDoNotWaitInDefaultMode(t *testing.T) {
	k := tracker(mode("default"), use("t1", "Read"))
	if got := k.Info(t0.Add(4 * time.Second)).Status; got != Working {
		t.Fatalf("got %v", got)
	}
}

func TestAcceptEditsExcludesEdits(t *testing.T) {
	if got := tracker(mode("acceptEdits"), use("t1", "Edit")).Info(t0.Add(4 * time.Second)).Status; got != Working {
		t.Fatalf("Edit: %v", got)
	}
	if got := tracker(mode("acceptEdits"), use("t1", "Bash")).Info(t0.Add(4 * time.Second)).Status; got != Waiting {
		t.Fatalf("Bash: %v", got)
	}
}

func TestWaitingExpiresAfterOneHour(t *testing.T) {
	k := tracker(mode("default"), use("t1", "Bash"))
	if got := k.Info(t0.Add(61 * time.Minute)).Status; got != Idle {
		t.Fatalf("got %v", got)
	}
}

func TestActiveThenIdle(t *testing.T) {
	k := tracker(asst("m1", t0, tr.Usage{}))
	if got := k.Info(t0.Add(10 * time.Second)).Status; got != Active {
		t.Fatalf("10s: %v", got)
	}
	if got := k.Info(t0.Add(40 * time.Second)).Status; got != Idle {
		t.Fatalf("40s: %v", got)
	}
}

func TestContextPctUsesLastAssistantMessage(t *testing.T) {
	k := NewTracker(200000)
	if k.Info(t0).CtxPct != -1 {
		t.Fatal("no data should give -1")
	}
	k.Apply([]tr.Event{
		asst("m1", t0, tr.Usage{Input: 1000}),
		asst("m2", t0, tr.Usage{Input: 1000, CacheCreate: 1000, CacheRead: 49000}),
	}, t0)
	in := k.Info(t0)
	if in.CtxTokens != 51000 || in.CtxPct != 25 || in.Model != "claude-sonnet-5-5" {
		t.Fatalf("%+v", in)
	}
	k.Apply([]tr.Event{asst("m3", t0, tr.Usage{Input: 900000})}, t0)
	if got := k.Info(t0).CtxPct; got != 90 { // more than the window set: it is a million-token model
		t.Fatalf("large window: %d", got)
	}
	k.Apply([]tr.Event{asst("m4", t0, tr.Usage{Input: 1200000})}, t0)
	if got := k.Info(t0).CtxPct; got != 100 {
		t.Fatalf("cap: %d", got)
	}
}

func TestTokensTodayDedupesByMessageID(t *testing.T) {
	yesterday := t0.Add(-24 * time.Hour)
	u := tr.Usage{Input: 10, CacheCreate: 20, CacheRead: 5000, Output: 30}
	k := tracker(asst("m1", t0, u), asst("m1", t0, u), asst("m2", yesterday, u))
	if got := k.Info(t0).TokensToday; got != 60 { // 10+20+30, cache reads not counted, once
		t.Fatalf("TokensToday = %d, want 60", got)
	}
}

func TestUnknownGarbageDoesNotPanic(t *testing.T) {
	k := NewTracker(0) // 0 falls back to the default window
	k.Apply([]tr.Event{result("nope"), {}, mode(""), use("", ""), asst("", time.Time{}, tr.Usage{})}, t0)
	_ = k.Info(t0)
	_ = k.Info(t0.Add(time.Hour))
}

func TestActivityRecordsToolsPromptsAndReplies(t *testing.T) {
	tk := tracker(
		tr.Event{Kind: tr.UserPrompt, Time: t0},
		use("t1", "Read"),
		asst("m1", t0, tr.Usage{}),
	)
	got := tk.Activity()
	if len(got) != 3 || got[0].Text != "reply" || got[1].Text != "tool: Read" || got[2].Text != "user prompt" {
		t.Fatalf("%+v", got)
	}
}

func TestActivityRepliesCountOncePerMessageID(t *testing.T) {
	tk := tracker(asst("m1", t0, tr.Usage{}), asst("m1", t0, tr.Usage{}))
	if n := len(tk.Activity()); n != 1 {
		t.Fatalf("got %d", n)
	}
}

func TestActivityKeepsLast50(t *testing.T) {
	var evs []tr.Event
	for i := 0; i < 60; i++ {
		evs = append(evs, use(fmt.Sprintf("t%d", i), fmt.Sprintf("Tool%d", i)))
	}
	got := tracker(evs...).Activity()
	if len(got) != 50 || got[0].Text != "tool: Tool59" || got[49].Text != "tool: Tool10" {
		t.Fatalf("len %d first %q last %q", len(got), got[0].Text, got[len(got)-1].Text)
	}
}

func TestAContextBeyondTheWindowMeansABiggerWindow(t *testing.T) {
	tk := NewTracker(200000)
	say := func(id string, n int) {
		tk.Apply([]tr.Event{{Kind: tr.Assistant, MessageID: id, Usage: tr.Usage{CacheRead: n}}}, time.Now())
	}
	say("a", 300000)
	if got := tk.Info(time.Now()).CtxPct; got != 30 {
		t.Fatalf("%d%%, want 30%% of a million", got)
	}
	say("b", 100000) // after a compaction it is still that model
	if got := tk.Info(time.Now()).CtxPct; got != 10 {
		t.Fatalf("%d%%", got)
	}
}
