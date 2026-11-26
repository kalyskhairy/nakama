// Package monitor turns a session's transcript events into what the screen shows:
// status, context use and tokens.
package monitor

import (
	"time"

	tr "github.com/kalyskhairy/nakama/internal/transcript"
)

type Status int

const (
	Idle Status = iota
	Active
	Working
	Waiting
)

type Info struct {
	Status      Status
	Model       string
	CtxTokens   int
	CtxPct      int // -1 until the first assistant message
	TokensToday int
	PendingTool string
	LastActive  time.Time // time of the last transcript event seen
	SentTo      string    // the last SendMessage: its receiver and when
	SentAt      time.Time
}

const (
	defaultWindow = 200000
	largeWindow   = 1000000
	waitAfter     = 3 * time.Second
	waitUntil     = time.Hour
	workingFor    = 2 * time.Minute
	activeFor     = 30 * time.Second
)

var (
	readOnly = map[string]bool{"Read": true, "Grep": true, "Glob": true, "LS": true, "WebSearch": true, "ToolSearch": true, "Skill": true}
	spawn    = map[string]bool{"Agent": true, "Task": true}
	edits    = map[string]bool{"Edit": true, "Write": true, "NotebookEdit": true}
)

// asks reports whether a pending tool call is waiting for the user's approval in this permission mode.
func asks(mode, tool string) bool {
	if tool == "AskUserQuestion" || tool == "ExitPlanMode" {
		return true
	}
	if mode == "auto" || mode == "bypassPermissions" {
		return false
	}
	if readOnly[tool] || spawn[tool] {
		return false
	}
	return !(mode == "acceptEdits" && edits[tool])
}

const maxActivity = 50

// Activity is one thing a session did: a prompt, a tool call or a reply. Never the tool's arguments.
type Activity struct {
	Time time.Time
	Text string
}

type tokenRec struct {
	at time.Time
	n  int
}

type Tracker struct {
	window    int
	mode      string
	pending   map[string]string // tool id -> tool name
	order     []string          // pending tool ids, oldest first
	lastEvent time.Time
	model     string
	ctx       int
	haveCtx   bool
	counted   map[string]bool
	tokens    []tokenRec
	done      bool       // the last thing it did was to finish: an end of turn, or a hand-back
	act       []Activity // oldest first, at most maxActivity
	sentTo    string
	sentAt    time.Time
}

func NewTracker(window int) *Tracker {
	if window <= 0 {
		window = defaultWindow
	}
	return &Tracker{window: window, pending: map[string]string{}, counted: map[string]bool{}}
}

func (t *Tracker) Apply(events []tr.Event, now time.Time) {
	if len(events) == 0 {
		return
	}
	t.lastEvent = now
	for _, e := range events {
		switch e.Kind {
		case tr.PermissionMode:
			if e.Mode != "" {
				t.mode = e.Mode
			}
		case tr.UserPrompt:
			t.done = false
			t.record(e.Time, now, "user prompt")
		case tr.ToolUse:
			t.done = false
			if e.To != "" {
				t.sentTo, t.sentAt = e.To, e.Time
			}
			if e.ToolID == "" {
				continue
			}
			if _, dup := t.pending[e.ToolID]; !dup {
				t.order = append(t.order, e.ToolID)
				t.record(e.Time, now, "tool: "+e.ToolName)
			}
			t.pending[e.ToolID] = e.ToolName
		case tr.ToolResult:
			t.done = t.pending[e.ToolID] == "SubagentHandback"
			delete(t.pending, e.ToolID)
			for i, id := range t.order {
				if id == e.ToolID {
					t.order = append(t.order[:i], t.order[i+1:]...)
					break
				}
			}
		case tr.Assistant:
			t.done = e.StopReason == "end_turn"
			if e.Model != "" {
				t.model = e.Model
			}
			t.ctx, t.haveCtx = e.Usage.Input+e.Usage.CacheCreate+e.Usage.CacheRead, true
			if t.ctx > t.window {
				t.window = max(t.ctx, largeWindow) // it cannot hold more than its window: this one is the large kind
			}
			if e.MessageID != "" && !t.counted[e.MessageID] {
				t.counted[e.MessageID] = true
				t.record(e.Time, now, "reply")
				at := e.Time
				if at.IsZero() {
					at = now
				}
				t.tokens = append(t.tokens, tokenRec{at, e.Usage.Input + e.Usage.CacheCreate + e.Usage.Output})
			}
		}
	}
}

func (t *Tracker) record(at, now time.Time, text string) {
	if at.IsZero() {
		at = now
	}
	t.act = append(t.act, Activity{at, text})
	if len(t.act) > maxActivity {
		t.act = t.act[len(t.act)-maxActivity:]
	}
}

// Activity lists what the session did, newest first.
func (t *Tracker) Activity() []Activity {
	out := make([]Activity, len(t.act))
	for i, a := range t.act {
		out[len(t.act)-1-i] = a
	}
	return out
}

// Done reports that a subagent has delivered its work; a later message makes it work again.
func (t *Tracker) Done() bool { return t.done && len(t.pending) == 0 }

// Busy reports a tool call that has no result yet.
func (t *Tracker) Busy() bool { return len(t.pending) > 0 }

func (t *Tracker) Mode() string         { return t.mode }
func (t *Tracker) SetMode(mode string)  { t.mode = mode }
func (t *Tracker) LastEvent() time.Time { return t.lastEvent }

func sameDay(a, b time.Time) bool {
	a = a.In(b.Location())
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

func (t *Tracker) Info(now time.Time) Info {
	in := Info{Model: t.model, CtxTokens: t.ctx, CtxPct: -1, LastActive: t.lastEvent, SentTo: t.sentTo, SentAt: t.sentAt}
	if t.haveCtx {
		in.CtxPct = min(100, t.ctx*100/t.window)
	}
	for _, r := range t.tokens {
		if sameDay(r.at, now) {
			in.TokensToday += r.n
		}
	}
	age := now.Sub(t.lastEvent)
	var asking string
	for _, id := range t.order {
		if name, ok := t.pending[id]; ok && asks(t.mode, name) {
			asking = name
			break
		}
	}
	switch {
	case len(t.pending) > 0 && asking != "" && age >= waitAfter && age < waitUntil:
		in.Status, in.PendingTool = Waiting, asking
	case len(t.pending) > 0 && age < workingFor:
		in.Status = Working
		for _, id := range t.order {
			if name, ok := t.pending[id]; ok {
				in.PendingTool = name
				break
			}
		}
	case age < activeFor:
		in.Status = Active
	}
	return in
}
