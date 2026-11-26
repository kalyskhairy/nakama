// Package index lists every Claude Code session on the machine from the transcripts
// under ~/.claude*/projects, reading only the start and the end of each file.
package index

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kalyskhairy/nakama/internal/limits"
)

const (
	headBytes = 64 << 10
	// shortcut: a 256 KB tail can miss the title when the last lines are huge; raise it if that shows up.
	tailBytes = 256 << 10
	maxTitle  = 60
)

type Entry struct {
	ID, Account, ConfigDir, Path, Cwd, Title string
	ModTime                                  time.Time
	Size                                     int64
}

type cached struct {
	size int64
	mod  time.Time
	e    Entry
}

type Indexer struct {
	roots func() []string
	cache map[string]cached
	read  func(path string, off int64, n int) ([]byte, error)
}

func NewIndexer(roots func() []string) *Indexer {
	return &Indexer{roots: roots, cache: map[string]cached{}, read: readAt}
}

func readAt(path string, off int64, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	got, err := f.ReadAt(buf, off)
	if err == io.EOF {
		err = nil
	}
	return buf[:got], err
}

// Scan returns every top-level transcript, newest first. A file is read again only when its size or
// modification time changed.
func (x *Indexer) Scan() []Entry {
	var out []Entry
	seen := map[string]bool{}
	for _, root := range x.roots() {
		files, _ := filepath.Glob(filepath.Join(root, "*", "*.jsonl"))
		for _, p := range files {
			st, err := os.Stat(p)
			if err != nil || !st.Mode().IsRegular() {
				continue
			}
			seen[p] = true
			c, ok := x.cache[p]
			if !ok || c.size != st.Size() || !c.mod.Equal(st.ModTime()) {
				cfg := filepath.Dir(root)
				e := Entry{ID: strings.TrimSuffix(filepath.Base(p), ".jsonl"), Account: limits.AccountName(cfg), ConfigDir: cfg,
					Path: p, ModTime: st.ModTime(), Size: st.Size()}
				x.fill(&e)
				c = cached{st.Size(), st.ModTime(), e}
				x.cache[p] = c
			}
			out = append(out, c.e)
		}
	}
	for p := range x.cache {
		if !seen[p] {
			delete(x.cache, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out
}

type line struct {
	Type        string `json:"type"`
	Cwd         string `json:"cwd"`
	AITitle     string `json:"aiTitle"`
	CustomTitle string `json:"customTitle"`
	Message     struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

func (x *Indexer) fill(e *Entry) {
	head, _ := x.read(e.Path, 0, headBytes)
	var prompt string
	for _, ln := range bytes.Split(head, []byte("\n")) {
		var l line
		if len(ln) == 0 || ln[0] != '{' || json.Unmarshal(ln, &l) != nil {
			continue
		}
		if e.Cwd == "" {
			e.Cwd = l.Cwd
		}
		if prompt == "" && l.Type == "user" {
			prompt = promptText(l.Message.Content)
		}
		if e.Cwd != "" && prompt != "" {
			break
		}
	}
	var ai, custom string
	off := max(e.Size-tailBytes, 0)
	tail, _ := x.read(e.Path, off, tailBytes)
	if off > 0 { // the first line of the tail is cut in half
		if i := bytes.IndexByte(tail, '\n'); i >= 0 {
			tail = tail[i+1:]
		}
	}
	for _, ln := range bytes.Split(tail, []byte("\n")) {
		if e.Cwd == "" && bytes.Contains(ln, []byte(`"cwd":"`)) { // the head had none: huge first lines
			var l line
			if json.Unmarshal(ln, &l) == nil {
				e.Cwd = l.Cwd
			}
		}
		if !bytes.Contains(ln, []byte(`-title"`)) {
			continue
		}
		var l line
		if json.Unmarshal(ln, &l) != nil {
			continue
		}
		if l.Type == "ai-title" && l.AITitle != "" {
			ai = l.AITitle
		}
		if l.Type == "custom-title" && l.CustomTitle != "" {
			custom = l.CustomTitle
		}
	}
	switch {
	case ai != "":
		e.Title = ai
	case custom != "":
		e.Title = custom
	case prompt != "":
		e.Title = prompt
	default:
		e.Title = e.ID[:min(8, len(e.ID))]
	}
}

// promptText is the text of a user message, skipping injected system text (which starts with "<").
func promptText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	var s string
	if len(raw) > 0 && raw[0] == '"' {
		json.Unmarshal(raw, &s)
	} else if len(raw) > 0 && raw[0] == '[' {
		var blocks []struct{ Type, Text string }
		json.Unmarshal(raw, &blocks)
		for _, b := range blocks {
			if b.Type == "text" && !strings.HasPrefix(strings.TrimSpace(b.Text), "<") {
				s = b.Text
				break
			}
		}
	}
	s = strings.Join(strings.Fields(s), " ")
	if strings.HasPrefix(s, "<") {
		return ""
	}
	if utf8.RuneCountInString(s) > maxTitle {
		s = string([]rune(s)[:maxTitle])
	}
	return s
}

type Folder struct {
	Cwd    string
	Count  int
	Latest time.Time
}

// Folders groups sessions by working directory, most recently used first. The same directory under
// two accounts is one folder.
func Folders(es []Entry) []Folder {
	by := map[string]*Folder{}
	var out []*Folder
	for _, e := range es {
		f := by[e.Cwd]
		if f == nil {
			f = &Folder{Cwd: e.Cwd}
			by[e.Cwd] = f
			out = append(out, f)
		}
		f.Count++
		if e.ModTime.After(f.Latest) {
			f.Latest = e.ModTime
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Latest.After(out[j].Latest) })
	res := make([]Folder, len(out))
	for i, f := range out {
		res[i] = *f
	}
	return res
}

// FolderName is the last part of a path, with / or \ as separator; "?" when there is none.
func FolderName(cwd string) string {
	cwd = strings.TrimRight(cwd, `/\`)
	if i := strings.LastIndexAny(cwd, `/\`); i >= 0 {
		cwd = cwd[i+1:]
	}
	if cwd == "" {
		return "?"
	}
	return cwd
}
