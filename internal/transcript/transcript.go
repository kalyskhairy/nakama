// Package transcript reads Claude Code's session transcripts (.jsonl) into events.
package transcript

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"time"
)

type Kind int

const (
	Assistant Kind = iota
	ToolUse
	ToolResult
	UserPrompt
	PermissionMode
)

type Usage struct{ Input, CacheCreate, CacheRead, Output int }

type Event struct {
	Kind       Kind
	Time       time.Time
	MessageID  string // Assistant
	Model      string // Assistant
	StopReason string // Assistant
	Usage      Usage  // Assistant
	ToolID     string // ToolUse, ToolResult
	ToolName   string // ToolUse
	To         string // ToolUse of SendMessage: who it was for (the message itself is not kept)
	Mode       string // PermissionMode
}

type block struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	ToolUseID string `json:"tool_use_id"`
	Text      string `json:"text"`
	Input     struct {
		To json.RawMessage `json:"to"`
	} `json:"input"`
}

type line struct {
	Type           string `json:"type"`
	Timestamp      string `json:"timestamp"`
	IsSidechain    bool   `json:"isSidechain"`
	PermissionMode string `json:"permissionMode"`
	Message        struct {
		ID         string `json:"id"`
		Model      string `json:"model"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			Input       int `json:"input_tokens"`
			CacheCreate int `json:"cache_creation_input_tokens"`
			CacheRead   int `json:"cache_read_input_tokens"`
			Output      int `json:"output_tokens"`
		} `json:"usage"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// ParseLine turns one transcript line into events. Unknown, broken or
// sidechain lines give nil.
func ParseLine(b []byte) []Event { return parseLine(b, false) }

// ParseLineSidechain is ParseLine for a subagent's own transcript, where every line is a sidechain line.
func ParseLineSidechain(b []byte) []Event { return parseLine(b, true) }

func parseLine(b []byte, sidechain bool) []Event {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || b[0] != '{' {
		return nil
	}
	var l line
	if json.Unmarshal(b, &l) != nil || (l.IsSidechain && !sidechain) {
		return nil
	}
	ts, _ := time.Parse(time.RFC3339Nano, l.Timestamp)
	var blocks []block
	content := bytes.TrimSpace(l.Message.Content)
	if len(content) > 0 && content[0] == '[' {
		json.Unmarshal(content, &blocks) // a broken block list just yields no blocks
	}
	switch l.Type {
	case "permission-mode":
		if l.PermissionMode == "" {
			return nil
		}
		return []Event{{Kind: PermissionMode, Time: ts, Mode: l.PermissionMode}}
	case "assistant":
		if l.Message.ID == "" {
			return nil
		}
		u := l.Message.Usage
		out := []Event{{Kind: Assistant, Time: ts, MessageID: l.Message.ID, Model: l.Message.Model,
			StopReason: l.Message.StopReason, Usage: Usage{u.Input, u.CacheCreate, u.CacheRead, u.Output}}}
		for _, bl := range blocks {
			if bl.Type == "tool_use" {
				e := Event{Kind: ToolUse, Time: ts, ToolID: bl.ID, ToolName: bl.Name}
				if bl.Name == "SendMessage" {
					json.Unmarshal(bl.Input.To, &e.To) // another tool's "to" may be anything
				}
				out = append(out, e)
			}
		}
		return out
	case "user":
		var out []Event
		for _, bl := range blocks {
			if bl.Type == "tool_result" {
				out = append(out, Event{Kind: ToolResult, Time: ts, ToolID: bl.ToolUseID})
			}
		}
		if len(out) > 0 {
			return out
		}
		if len(content) > 0 && (content[0] == '"' || len(blocks) > 0) {
			return []Event{{Kind: UserPrompt, Time: ts}}
		}
	}
	return nil
}

// Reader returns the events appended to a transcript since the last Poll.
type Reader struct {
	path      string
	offset    int64
	partial   []byte
	sidechain bool
	tail      int64 // first Poll starts at most this many bytes from the end (0 = from the start)
	started   bool
	skipFirst bool // the first line after a tail start is cut in half
}

func NewReader(path string) *Reader { return &Reader{path: path} }

// NewTailReader reads a long-running transcript: the first Poll only looks at the last max bytes,
// so opening a session of hundreds of MB does not read all of it.
func NewTailReader(path string, max int64) *Reader { return &Reader{path: path, tail: max} }

// NewSidechainReader reads a subagent's transcript, whose lines are all sidechain lines.
func NewSidechainReader(path string) *Reader { return &Reader{path: path, sidechain: true} }

func (r *Reader) Poll() ([]Event, error) {
	f, err := os.Open(r.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() < r.offset { // truncated or replaced: start over
		r.offset, r.partial = 0, nil
	}
	if !r.started {
		r.started = true
		if r.tail > 0 && st.Size() > r.tail {
			r.offset, r.skipFirst = st.Size()-r.tail, true
		}
	}
	if _, err := f.Seek(r.offset, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	r.offset += int64(len(data))
	if r.skipFirst {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			return nil, nil // still inside the cut line; keep skipping
		}
		data, r.skipFirst = data[i+1:], false
	}
	data = append(r.partial, data...)
	end := bytes.LastIndexByte(data, '\n')
	if end < 0 {
		r.partial = data
		return nil, nil
	}
	r.partial = append([]byte(nil), data[end+1:]...)
	var out []Event
	for _, ln := range bytes.Split(data[:end], []byte("\n")) {
		out = append(out, parseLine(ln, r.sidechain)...)
	}
	return out, nil
}
