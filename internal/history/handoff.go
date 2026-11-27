package history

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
)

const (
	handoffTail = 8 << 20  // read at most this much of the end of a transcript
	handoffKeep = 60 << 10 // and keep at most this much of the conversation
)

// Conversation is what the user and Claude said in a session, newest last, as Markdown: no tool
// calls, tool output, thinking or text the harness injected. Only the end of a long session is kept.
func Conversation(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	cut := st.Size() > handoffTail
	if cut {
		if _, err := f.Seek(st.Size()-handoffTail, io.SeekStart); err != nil {
			return "", err
		}
	}
	r := bufio.NewReaderSize(f, 1<<16)
	if cut { // the first line is cut in half
		if _, err := r.ReadBytes('\n'); err != nil {
			return "", err
		}
	}
	var blocks []string
	for {
		b, err := r.ReadBytes('\n')
		var l line
		if len(b) > 0 && json.Unmarshal(b, &l) == nil && !l.IsMeta && !l.Sidechain && (l.Type == "user" || l.Type == "assistant") {
			if text := said(&l); text != "" {
				who := "User"
				if l.Type == "assistant" {
					who = "Claude"
				}
				blocks = append(blocks, "## "+who+"\n\n"+text+"\n")
			}
		}
		if err != nil {
			break
		}
	}
	size, first := 0, len(blocks)
	for first > 0 && size+len(blocks[first-1])+1 <= handoffKeep { // +1: the line between blocks
		first--
		size += len(blocks[first]) + 1
	}
	out := strings.Join(blocks[first:], "\n")
	if cut || first > 0 {
		out = "(the earlier part of the conversation is left out)\n\n" + out
	}
	return out, nil
}

// said is the text of a message without the parts the harness put in (they start with "<").
func said(l *line) string {
	var parts []string
	c := strings.TrimSpace(string(l.Message.Content))
	if strings.HasPrefix(c, `"`) {
		var s string
		json.Unmarshal(l.Message.Content, &s)
		parts = []string{s}
	} else {
		var blocks []struct{ Type, Text string }
		json.Unmarshal(l.Message.Content, &blocks)
		for _, b := range blocks {
			if b.Type == "text" {
				parts = append(parts, b.Text)
			}
		}
	}
	var keep []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" && !strings.HasPrefix(p, "<") {
			keep = append(keep, p)
		}
	}
	return strings.Join(keep, "\n\n")
}
