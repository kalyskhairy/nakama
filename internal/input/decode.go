package input

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

type EventKind int

const (
	EvKey EventKind = iota
	EvMouse
	EvCellSize // the terminal's reply to ESC [ 16 t: X and Y are the cell width and height in pixels
)

// Event is one thing the user did. Raw holds the bytes it came from, to hand to the child unchanged.
// For a mouse event X and Y are 0-based cells and Btn is 0 (left), 64 (wheel up) or 65 (wheel down).
type Event struct {
	Kind  EventKind
	Key   string // "" for a sequence nakama has no name for
	Raw   []byte
	X, Y  int
	Btn   int
	Press bool
}

var csiKeys = map[byte]string{'A': "up", 'B': "down", 'C': "right", 'D': "left", 'Z': "shift-tab"}
var tildeKeys = map[string]string{"5": "pgup", "6": "pgdn", "11": "f1"}

// DecodeOne reads one event from the start of b and says how many bytes it used (at least 1).
// A sequence that ends before its final byte is taken whole as an unnamed key, so it is never
// split or held back. shortcut: a sequence split across two reads is passed on as two pieces.
func DecodeOne(b []byte, toggle byte) (Event, int) {
	ev := func(key string, n int) (Event, int) { return Event{Kind: EvKey, Key: key, Raw: b[:n]}, n }
	switch c := b[0]; {
	case c == toggle:
		return ev("toggle", 1)
	case c == 0x1b:
		return decodeEsc(b, ev)
	case c == '\r' || c == '\n':
		return ev("enter", 1)
	case c == '\t':
		return ev("tab", 1)
	case c == 0x7f || c == 0x08:
		return ev("backspace", 1)
	case c == 0x03:
		return ev("ctrl-c", 1)
	case c < 0x20:
		return ev("", 1)
	}
	r, n := utf8.DecodeRune(b)
	if r == utf8.RuneError && n <= 1 {
		return ev("", 1)
	}
	return ev(string(r), n)
}

func decodeEsc(b []byte, ev func(string, int) (Event, int)) (Event, int) {
	if len(b) == 1 {
		return ev("esc", 1)
	}
	switch b[1] {
	case 'O':
		if len(b) < 3 {
			return ev("", len(b))
		}
		if b[2] == 'P' {
			return ev("f1", 3)
		}
		return ev(csiKeys[b[2]], 3)
	case '[':
		for i := 2; i < len(b); i++ {
			if b[i] < 0x40 || b[i] > 0x7e {
				continue
			}
			params := string(b[2:i])
			if m, ok := mouse(b[:i+1], params, b[i]); ok {
				return m, i + 1
			}
			if w, h, ok := cellSize(params, b[i]); ok {
				return Event{Kind: EvCellSize, Raw: b[:i+1], X: w, Y: h}, i + 1
			}
			switch {
			case b[i] == '~':
				return ev(tildeKeys[params], i+1)
			case params == "":
				return ev(csiKeys[b[i]], i+1)
			}
			return ev("", i+1)
		}
		return ev("", len(b))
	}
	return ev("", 2)
}

// cellSize parses the reply ESC [ 6 ; height ; width t.
func cellSize(params string, final byte) (w, h int, ok bool) {
	f := strings.Split(params, ";")
	if final != 't' || len(f) != 3 || f[0] != "6" {
		return 0, 0, false
	}
	h, e1 := strconv.Atoi(f[1])
	w, e2 := strconv.Atoi(f[2])
	return w, h, e1 == nil && e2 == nil && w > 0 && h > 0
}

// mouse parses an SGR mouse report: ESC [ < button ; x ; y (M press | m release).
func mouse(raw []byte, params string, final byte) (Event, bool) {
	if !strings.HasPrefix(params, "<") || (final != 'M' && final != 'm') {
		return Event{}, false
	}
	f := strings.Split(params[1:], ";")
	if len(f) != 3 {
		return Event{}, false
	}
	btn, e1 := strconv.Atoi(f[0])
	x, e2 := strconv.Atoi(f[1])
	y, e3 := strconv.Atoi(f[2])
	if e1 != nil || e2 != nil || e3 != nil {
		return Event{}, false
	}
	return Event{Kind: EvMouse, Raw: raw, X: x - 1, Y: y - 1, Btn: btn &^ (4 | 8 | 16), Press: final == 'M'}, true
}
