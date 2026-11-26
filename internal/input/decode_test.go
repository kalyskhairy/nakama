package input

import (
	"strings"
	"testing"
)

func TestDecodeOne(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		toggle byte
		key    string
		n      int
	}{
		{"bare esc", "\x1b", 0, "esc", 1},
		{"arrow up", "\x1b[A", 0, "up", 3},
		{"arrow down then more", "\x1b[Bxyz", 0, "down", 3},
		{"shift-tab", "\x1b[Z", 0, "shift-tab", 3},
		{"ctrl-right keeps raw", "\x1b[1;5C", 0, "", 6},
		{"f1 ss3", "\x1bOP", 0, "f1", 3},
		{"f1 csi", "\x1b[11~", 0, "f1", 5},
		{"pgup", "\x1b[5~", 0, "pgup", 4},
		{"pgdn", "\x1b[6~", 0, "pgdn", 4},
		{"ctrl-space toggle", "\x00", 0, "toggle", 1},
		{"custom toggle", "\x1d", 0x1d, "toggle", 1},
		{"nul is no toggle when another key is set", "\x00", 0x1d, "", 1},
		{"utf8 rune", "é!", 0, "é", 2},
		{"letter", "q", 0, "q", 1},
		{"enter", "\r", 0, "enter", 1},
		{"tab", "\t", 0, "tab", 1},
		{"backspace", "\x7f", 0, "backspace", 1},
		{"ctrl-c", "\x03", 0, "ctrl-c", 1},
		{"alt-x stays whole", "\x1bx", 0, "", 2},
		{"truncated csi does not hang", "\x1b[1;5", 0, "", 5},
		{"invalid utf8", "\xff", 0, "", 1},
	}
	for _, c := range cases {
		ev, n := DecodeOne([]byte(c.in), c.toggle)
		if ev.Kind != EvKey || ev.Key != c.key || n != c.n || string(ev.Raw) != c.in[:c.n] {
			t.Errorf("%s: got key %q n %d raw %q", c.name, ev.Key, n, ev.Raw)
		}
	}
}

func TestDecodeMouse(t *testing.T) {
	ev, n := DecodeOne([]byte("\x1b[<64;10;5M"), 0)
	if ev.Kind != EvMouse || ev.Btn != 64 || ev.X != 9 || ev.Y != 4 || !ev.Press || n != 11 {
		t.Fatalf("%+v n=%d", ev, n)
	}
	ev, _ = DecodeOne([]byte("\x1b[<0;3;4m"), 0)
	if ev.Kind != EvMouse || ev.Btn != 0 || ev.Press || ev.X != 2 || ev.Y != 3 {
		t.Fatalf("%+v", ev)
	}
	ev, _ = DecodeOne([]byte("\x1b[<20;3;4M"), 0) // ctrl+left: modifier bits are dropped
	if ev.Btn != 0 {
		t.Fatalf("%+v", ev)
	}
}

func TestDecodeWalksAWholeChunk(t *testing.T) {
	data := []byte("ab\x1b[A\x1b[<0;1;1M\x1b")
	var kinds []string
	for len(data) > 0 {
		ev, n := DecodeOne(data, 0)
		kinds = append(kinds, ev.Key+map[EventKind]string{EvMouse: "mouse"}[ev.Kind])
		data = data[n:]
	}
	if got := strings.Join(kinds, ","); got != "a,b,up,mouse,esc" {
		t.Fatal(got)
	}
}

func TestBindingsMentionEveryDocumentedKey(t *testing.T) {
	var all strings.Builder
	for _, b := range Bindings {
		all.WriteString(b.Keys + " " + b.Desc + "\n")
	}
	for _, want := range []string{"Ctrl+Space", "F1", "z", "f", "n", "d", "v", "x", "q", "m", "/", "Enter", "Tab"} {
		found := false
		for _, b := range Bindings {
			for _, k := range strings.Split(b.Keys, " / ") {
				if k == want {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("no binding for %q", want)
		}
	}
}

func TestDecodeCellSizeReport(t *testing.T) {
	ev, n := DecodeOne([]byte("\x1b[6;20;10t"), 0)
	if ev.Kind != EvCellSize || ev.X != 10 || ev.Y != 20 || n != 10 {
		t.Fatalf("%+v n=%d", ev, n)
	}
}
