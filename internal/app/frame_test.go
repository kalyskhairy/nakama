package app

import (
	"github.com/kalyskhairy/nakama/internal/input"
	"github.com/kalyskhairy/nakama/internal/monitor"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

var rowMark = regexp.MustCompile(`\x1b\[(\d+);1H`)

// plainRows splits a frame into screen rows (1-based row marks), stripped of ANSI.
func plainRows(frame string, h int) []string {
	rows := make([]string, h)
	idx := rowMark.FindAllStringSubmatchIndex(frame, -1)
	for i, m := range idx {
		r, _ := strconv.Atoi(frame[m[2]:m[3]])
		end := len(frame)
		if i+1 < len(idx) {
			end = idx[i+1][0]
		}
		if r >= 1 && r <= h {
			rows[r-1] = ansi.Strip(frame[m[1]:end])
		}
	}
	return rows
}

func containsRow(rows []string, want string) bool {
	for _, r := range rows {
		if strings.Contains(r, want) {
			return true
		}
	}
	return false
}

func chatModel(t *testing.T, script string) *Model {
	t.Helper()
	m := newModel(t, spawnCmd("sh", "-c", script))
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	return m
}

func TestFrameChatShowsSessionOutputAndStatusLine(t *testing.T) {
	m := chatModel(t, "printf hello; sleep 30")
	waitFor(t, "hello", func() bool { return strings.Contains(m.Sessions()[0].Text(), "hello") })
	rows := plainRows(Frame(m, 80, 25, t0), 25)
	if !containsRow(rows[:24], "hello") {
		t.Errorf("session output missing: %q", rows)
	}
	if !strings.Contains(rows[24], "F1 bantuan") {
		t.Errorf("status line: %q", rows[24])
	}
}

func TestFrameWorkEmptyShowsPanels(t *testing.T) {
	m := newModel(t, spawnCmd("sleep", "30"))
	rows := plainRows(Frame(m, 140, 40, t0), 40)
	for _, want := range []string{"Aktif (0)", "Folder / Sesi", "Chat", "Enter membuka sesi terpilih"} {
		if !containsRow(rows, want) {
			t.Errorf("no %q in %q", want, rows)
		}
	}
}

func TestFrameTooSmall(t *testing.T) {
	m := newModel(t, spawnCmd("sleep", "30"))
	if got := Frame(m, 30, 8, t0); !strings.Contains(ansi.Strip(got), "terminal terlalu kecil") {
		t.Errorf("got %q", got)
	}
	_ = Frame(m, 0, 0, t0) // must not panic
}

func TestFrameAfterResizeToTinyAndBack(t *testing.T) {
	m := chatModel(t, "printf hello; sleep 30")
	for _, sz := range [][2]int{{10, 3}, {0, 0}, {1, 1}, {100, 30}} {
		m.Resize(sz[0], sz[1])
		_ = Frame(m, sz[0], sz[1], t0) // must not panic
	}
	rows := plainRows(Frame(m, 100, 30, t0), 30)
	if !strings.Contains(rows[29], "F1 bantuan") {
		t.Errorf("status line after resize: %q", rows[29])
	}
}

func TestFrameCursorFollowsFocusedSession(t *testing.T) {
	m := chatModel(t, "printf abc; sleep 30")
	waitFor(t, "abc", func() bool { return strings.Contains(m.Sessions()[0].Text(), "abc") })
	if got := Frame(m, 80, 25, t0); !strings.HasSuffix(got, "\x1b[2;4H\x1b[?25h") {
		t.Errorf("chat cursor: %q", got[max(0, len(got)-30):])
	}
	m.HandleEvent(key("toggle"), t0)
	if got := Frame(m, 80, 25, t0); !strings.HasSuffix(got, "\x1b[?25l") {
		t.Errorf("home cursor: %q", got[max(0, len(got)-30):])
	}
}

func TestFrameErasesStaleText(t *testing.T) {
	m := chatModel(t, `printf hello; sleep 0.4; printf '\b\b\b\033[K'; sleep 30`)
	row := func(s string, n int) string { return strings.TrimRight(strings.Split(s, "\n")[n], " ") }
	waitFor(t, "hello", func() bool { return row(m.Sessions()[0].Text(), 0) == "hello" })
	term := vt.NewEmulator(80, 25) // stands in for the real terminal
	term.Write([]byte(Frame(m, 80, 25, t0)))
	waitFor(t, "he", func() bool { return row(m.Sessions()[0].Text(), 0) == "he" })
	term.Write([]byte(Frame(m, 80, 25, t0)))
	if got := row(ansi.Strip(term.Render()), 1); got != "he" { // the chat starts under its header row
		t.Fatalf("terminal row 0 = %q, want %q (old text left on screen)", got, "he")
	}
}

func ansiStrip(s string) string { return ansi.Strip(s) }

func TestFrameWorkLayoutShowsPanelsStripAndStatus(t *testing.T) {
	m, _, fm := workModel(t, []string{"/p"}, ent("a", "/p", "Judul sesi", time.Minute))
	fm.info = map[string]monitor.Info{"a": {Status: monitor.Working, Model: "claude-opus-4", CtxPct: 41, PendingTool: "Read"}}
	selectSession(m, "a")
	rows := plainRows(Frame(m, 140, 40, t0), 40)
	for _, want := range []string{"Aktif (1)", "Folder / Sesi", "Judul sesi", "opus-4 · bekerja · konteks 41%", "alat: Read", "Chat ──", "Enter membuka sesi terpilih"} {
		if !containsRow(rows[:39], want) {
			t.Errorf("no %q in %q", want, rows[:39])
		}
	}
	if !strings.Contains(rows[39], "F1 bantuan") {
		t.Errorf("status row: %q", rows[39])
	}
}

func TestChatPtyGetsPanelSizeAndZoomChangesIt(t *testing.T) {
	script := `trap 'stty size' WINCH; stty size; while :; do sleep 0.1; done`
	m := newModel(t, spawnCmd("sh", "-c", script))
	m.newSession(SpawnReq{Dir: "/p"}, t0)
	screen := func() string { return m.Sessions()[0].Text() }
	waitFor(t, "panel size 36 105", func() bool { return strings.Contains(screen(), "36 105") })
	m.HandleEvent(key("toggle"), t0)
	m.HandleEvent(key("z"), t0)
	if m.Mode() != input.Chat {
		t.Fatalf("z must open the zoomed chat: %v", m.Mode())
	}
	waitFor(t, "zoom size 38 140", func() bool { return strings.Contains(screen(), "38 140") })
	m.HandleEvent(key("toggle"), t0)
	if m.Mode() != input.Work {
		t.Fatalf("Ctrl+Space must leave the zoom: %v", m.Mode())
	}
	waitFor(t, "back to 36 105", func() bool { return strings.HasSuffix(strings.TrimSpace(screen()), "36 105") })
}

func TestZoomHidesTheLeftColumn(t *testing.T) {
	m := chatModel(t, "sleep 30")
	m.HandleEvent(key("toggle"), t0)
	m.HandleEvent(key("z"), t0)
	rows := plainRows(Frame(m, 140, 40, t0), 40)
	if containsRow(rows, "Aktif (") || containsRow(rows, "Folder / Sesi") || !strings.Contains(rows[0], "Chat ──") {
		t.Fatalf("zoomed frame: %q", rows[:3])
	}
}

func TestNarrowTerminalShowsOnlyTheFocusedColumn(t *testing.T) {
	m := chatModel(t, "sleep 30")
	m.Resize(80, 25)
	rows := plainRows(Frame(m, 80, 25, t0), 25)
	if containsRow(rows, "Aktif (") || !strings.Contains(rows[0], "Chat ──") {
		t.Fatalf("chat focus on a narrow screen: %q", rows[:3])
	}
	m.HandleEvent(key("toggle"), t0)
	rows = plainRows(Frame(m, 80, 25, t0), 25)
	if !containsRow(rows, "Aktif (") || containsRow(rows, "Chat ──") {
		t.Fatalf("list focus on a narrow screen: %q", rows[:3])
	}
}

func TestOverlaysAreDrawnOverThePanels(t *testing.T) {
	m, _, _ := workModel(t, []string{"/p"}, ent("a", "/p", "Judul", time.Hour))
	cases := []struct {
		key  string
		want []string
	}{{"f1", []string{"Bantuan", "Daftar", "Tab"}}, {"f", []string{"Folder yang ditampilkan", "[x]"}}, {"n", []string{"Sesi baru", "Akun:"}}, {"d", []string{"Detail", "Judul"}}}
	for _, c := range cases {
		m.mode, m.overlay = input.Work, ovNone
		if c.key == "d" {
			selectSession(m, "a")
		}
		if c.key == "f1" {
			m.HandleEvent(input.Event{Kind: input.EvKey, Key: "f1"}, t0)
		} else {
			m.HandleEvent(key(c.key), t0)
		}
		if m.Mode() != input.Overlay {
			t.Fatalf("%s: mode %v", c.key, m.Mode())
		}
		rows := plainRows(Frame(m, 140, 40, t0), 40)
		for _, w := range c.want {
			if !containsRow(rows, w) {
				t.Errorf("%s: no %q in the frame", c.key, w)
			}
		}
		m.HandleEvent(key("esc"), t0)
		if m.Mode() != input.Work {
			t.Fatalf("%s: esc left mode %v", c.key, m.Mode())
		}
	}
}

func TestMouseSequenceEmittedOnlyWhenTheStateChanges(t *testing.T) {
	m := newModel(t, spawnCmd("sleep", "30"))
	if got := m.MouseSeq(); !strings.Contains(got, "?1000h") || !strings.Contains(got, "?1006h") {
		t.Fatalf("first call must turn the mouse on: %q", got)
	}
	if got := m.MouseSeq(); got != "" {
		t.Fatalf("unchanged state sent %q", got)
	}
	m.HandleEvent(key("m"), t0)
	if got := m.MouseSeq(); !strings.Contains(got, "?1000l") || !strings.Contains(got, "?1006l") {
		t.Fatalf("m must turn the mouse off: %q", got)
	}
	if got := m.MouseSeq(); got != "" {
		t.Fatalf("unchanged state sent %q", got)
	}
}
