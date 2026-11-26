// Package history answers questions about all transcripts: tokens used per day and session, and
// where something was said. It reads whole files, so callers run it in the background.
package history

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kalyskhairy/nakama/internal/limits"
)

type line struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
	IsMeta    bool   `json:"isMeta"`
	Message   struct {
		ID      string          `json:"id"`
		Content json.RawMessage `json:"content"`
		Usage   *struct {
			Input       int `json:"input_tokens"`
			CacheCreate int `json:"cache_creation_input_tokens"`
			Output      int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// text is what the user typed or Claude said: not tool calls, their results or thinking.
func (l *line) text() string {
	c := bytes.TrimSpace(l.Message.Content)
	if len(c) == 0 {
		return ""
	}
	if c[0] == '"' {
		var s string
		json.Unmarshal(c, &s)
		return s
	}
	var blocks []struct{ Type, Text string }
	json.Unmarshal(c, &blocks)
	var out []string
	for _, b := range blocks {
		if b.Type == "text" {
			out = append(out, b.Text)
		}
	}
	return strings.Join(out, " ")
}

// each calls f for every line of a file; lines can be many megabytes long.
func each(path string, f func([]byte)) {
	fh, err := os.Open(path)
	if err != nil {
		return
	}
	defer fh.Close()
	r := bufio.NewReaderSize(fh, 1<<16)
	for {
		b, err := r.ReadBytes('\n')
		if len(b) > 0 {
			f(b)
		}
		if err != nil {
			return
		}
	}
}

// files lists the transcripts under a projects root with the session each belongs to.
func files(root string, subagents bool, since time.Time, f func(path, session string)) {
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		parts := strings.Split(rel, string(filepath.Separator))
		switch {
		case len(parts) == 2: // project/session.jsonl
		case subagents && len(parts) == 4 && parts[2] == "subagents": // project/session/subagents/agent.jsonl
		default:
			return nil
		}
		if info, err := d.Info(); err != nil || info.ModTime().Before(since) {
			return nil
		}
		f(path, strings.TrimSuffix(parts[1], ".jsonl"))
		return nil
	})
}

func account(root string) string { return limits.AccountName(filepath.Dir(root)) }

// SessionUse is what one session used on one day.
type SessionUse struct {
	ID, Account, Cwd string
	Tokens           int
}

type Report struct {
	Days  map[string]int                    // "2006-01-02" -> tokens
	byDay map[string]map[string]*SessionUse // day -> session id
}

// Sessions lists who used tokens on a day, the heaviest first.
func (r Report) Sessions(day string) []SessionUse {
	var out []SessionUse
	for _, s := range r.byDay[day] {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tokens != out[j].Tokens {
			return out[i].Tokens > out[j].Tokens
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Usage adds up the tokens (input, cache writes and output, like the rest of nakama) since a time.
// A message counts once, however many lines or files repeat it.
func Usage(roots []string, since time.Time, loc *time.Location) Report {
	r := Report{Days: map[string]int{}, byDay: map[string]map[string]*SessionUse{}}
	y, mon, d := since.In(loc).Date()
	since = time.Date(y, mon, d, 0, 0, 0, 0, loc) // whole days
	seen := map[string]bool{}
	for _, root := range roots {
		files(root, true, since, func(path, session string) {
			each(path, func(b []byte) {
				if !bytes.Contains(b, []byte(`"usage"`)) {
					return
				}
				var l line
				if json.Unmarshal(b, &l) != nil || l.Type != "assistant" || l.Message.Usage == nil || l.Message.ID == "" || seen[l.Message.ID] {
					return
				}
				at, err := time.Parse(time.RFC3339Nano, l.Timestamp)
				if err != nil || at.Before(since) {
					return
				}
				seen[l.Message.ID] = true
				u := l.Message.Usage
				n, day := u.Input+u.CacheCreate+u.Output, at.In(loc).Format("2006-01-02")
				r.Days[day] += n
				if r.byDay[day] == nil {
					r.byDay[day] = map[string]*SessionUse{}
				}
				s := r.byDay[day][session]
				if s == nil {
					s = &SessionUse{ID: session, Account: account(root), Cwd: l.Cwd}
					r.byDay[day][session] = s
				}
				s.Tokens += n
			})
		})
	}
	return r
}

// Hit is a session in which the query was said.
type Hit struct {
	ID, Account, Cwd string
	At               time.Time // of the latest match
	Snippet          string    // around the latest match
	Count            int       // matching messages
}

func snippet(text, q string) string {
	text = strings.Join(strings.Fields(text), " ")
	i := strings.Index(strings.ToLower(text), q)
	if i < 0 || i > len(text) {
		i = 0
	}
	from, to := max(i-40, 0), min(i+len(q)+80, len(text))
	for from > 0 && !utf8.RuneStart(text[from]) {
		from--
	}
	for to < len(text) && !utf8.RuneStart(text[to]) {
		to++
	}
	return text[from:to]
}

// Search finds the sessions in which the query was typed or said, the latest first. Case does not
// matter. shortcut: a query with quotes or backslashes is stored escaped and is not found.
func Search(roots []string, query string, limit int) []Hit {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	var out []Hit
	for _, root := range roots {
		files(root, false, time.Time{}, func(path, session string) {
			h := Hit{ID: session, Account: account(root)}
			each(path, func(b []byte) {
				if h.Cwd != "" && !bytes.Contains(bytes.ToLower(b), []byte(q)) {
					return
				}
				var l line
				if json.Unmarshal(b, &l) != nil {
					return
				}
				if h.Cwd == "" {
					h.Cwd = l.Cwd
				}
				if (l.Type != "user" && l.Type != "assistant") || l.IsMeta {
					return
				}
				text := l.text()
				if !strings.Contains(strings.ToLower(text), q) {
					return
				}
				h.Count++
				at, _ := time.Parse(time.RFC3339Nano, l.Timestamp)
				if !at.Before(h.At) {
					h.At, h.Snippet = at, snippet(text, q)
				}
			})
			if h.Count > 0 {
				out = append(out, h)
			}
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.After(out[j].At)
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
