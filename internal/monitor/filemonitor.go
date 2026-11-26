package monitor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kalyskhairy/nakama/internal/transcript"
)

const (
	maxSubagents  = 12
	subagentQuiet = time.Minute // a subagent with nothing running and quiet this long is resting
)

type subEntry struct {
	reader    *transcript.Reader
	tracker   *Tracker
	typ, desc string
	finished  bool
}

type entry struct {
	path    string
	reader  *transcript.Reader
	tracker *Tracker
	subs    map[string]*subEntry
}

// SubInfo describes one subagent of a session.
type SubInfo struct {
	ID, Type, Description string
	Info                  Info
	LastActive            time.Time
}

// FileMonitor follows the transcripts of sessions started with a known Claude session id,
// and of the subagents they spawn. It is not safe for concurrent use; the caller serialises access.
type FileMonitor struct {
	roots  func() []string
	window int
	byID   map[string]*entry
	// TailBytes bounds the first read of a session's own transcript, which may be hundreds of MB.
	TailBytes int64
}

func NewFileMonitor(roots func() []string, window int) *FileMonitor {
	return &FileMonitor{roots: roots, window: window, byID: map[string]*entry{}, TailBytes: 4 << 20}
}

// Poll reads what the session's transcript and its subagents' transcripts gained since the last
// call. The file does not exist until the session gets its first prompt, so a missing file is not an error.
func (f *FileMonitor) Poll(id string, now time.Time) {
	e := f.byID[id]
	if e == nil {
		path := Locate(f.roots(), id)
		if path == "" {
			return
		}
		e = &entry{path: path, reader: transcript.NewTailReader(path, f.TailBytes), tracker: NewTracker(f.window), subs: map[string]*subEntry{}}
		f.byID[id] = e
	}
	if events, err := e.reader.Poll(); err == nil {
		e.tracker.Apply(events, now)
	}
	f.pollSubagents(e, now)
}

func (f *FileMonitor) pollSubagents(e *entry, now time.Time) {
	dir := strings.TrimSuffix(e.path, ".jsonl") + string(filepath.Separator) + "subagents"
	files, _ := filepath.Glob(filepath.Join(dir, "agent-*.jsonl"))
	for _, file := range files {
		aid := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(file), "agent-"), ".jsonl")
		sub := e.subs[aid]
		if sub == nil {
			sub = &subEntry{reader: transcript.NewSidechainReader(file), tracker: NewTracker(f.window)}
			e.subs[aid] = sub
		}
		sub.readMeta(strings.TrimSuffix(file, ".jsonl") + ".meta.json")
		sub.tracker.SetMode(e.tracker.Mode()) // a subagent asks permission the way its parent does
		if events, err := sub.reader.Poll(); err == nil {
			sub.tracker.Apply(events, now)
		}
	}
}

func (s *subEntry) readMeta(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var m struct {
		AgentType   string `json:"agentType"`
		Description string `json:"description"`
		Finished    bool   `json:"finished"`
	}
	if json.Unmarshal(b, &m) != nil {
		return
	}
	s.typ, s.desc, s.finished = m.AgentType, m.Description, m.Finished
}

func (f *FileMonitor) Info(id string, now time.Time) Info {
	if e := f.byID[id]; e != nil {
		return e.tracker.Info(now)
	}
	return NewTracker(f.window).Info(now)
}

// Activity lists what a session did, newest first; nil for an id not followed yet.
func (f *FileMonitor) Activity(id string) []Activity {
	if e := f.byID[id]; e != nil {
		return e.tracker.Activity()
	}
	return nil
}

// Subagents lists the subagents of a session that are working or waiting. A finished or resting one
// is not listed. At most 12, by id.
func (f *FileMonitor) Subagents(sessionID string, now time.Time) []SubInfo {
	e := f.byID[sessionID]
	if e == nil {
		return nil
	}
	var out []SubInfo
	for aid, s := range e.subs {
		if s.finished || s.tracker.Done() {
			continue
		}
		in := s.tracker.Info(now)
		if in.Status == Idle { // resting subagents leave, or they would crowd the ship
			switch age := now.Sub(s.tracker.LastEvent()); {
			case s.tracker.Busy() && age < waitUntil:
				in.Status = Working
			case age < subagentQuiet:
				in.Status = Active
			default:
				continue
			}
		}
		out = append(out, SubInfo{ID: aid, Type: s.typ, Description: s.desc, Info: in, LastActive: s.tracker.LastEvent()})
	}
	if len(out) > maxSubagents {
		sort.Slice(out, func(i, j int) bool {
			if !out[i].LastActive.Equal(out[j].LastActive) {
				return out[i].LastActive.After(out[j].LastActive)
			}
			return out[i].ID < out[j].ID
		})
		out = out[:maxSubagents]
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
