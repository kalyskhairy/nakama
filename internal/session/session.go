// Package session runs one program (normally `claude`) in a PTY and mirrors
// its screen into an embedded terminal emulator.
package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
)

type Config struct {
	Cmd  string
	Args []string
	Dir  string
	W, H int
	ID   string   // Claude session id this session was started with (--session-id), if any
	Env  []string // extra KEY=VALUE entries for the child; a bare KEY removes the inherited one
}

type Session struct {
	emu      *vt.SafeEmulator
	ptmx     *os.File
	cmd      *exec.Cmd
	title    string
	id       string
	exited   atomic.Bool
	closer   sync.Once
	onChange func()

	bracketed atomic.Bool  // the child asked for bracketed paste (mode 2004)
	mouse     atomic.Int32 // one bit per mouse tracking mode (1000, 1002, 1003) the child has on

	wmu   sync.Mutex // guards wbuf: input waiting to be written to the child
	wbuf  []byte
	wsig  chan struct{}
	wdone chan struct{}
}

func Start(cfg Config, onChange func()) (*Session, error) {
	dir := cfg.Dir
	if dir == "" || dir == "." {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		dir = wd
	}
	c := exec.Command(cfg.Cmd, cfg.Args...)
	c.Dir = dir
	// claude marks its redraws only for terminals it knows; the emulator needs the marks (see syncHold)
	c.Env = childEnv(append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor", "CLAUDE_CODE_FORCE_SYNC_OUTPUT=1"), cfg.Env)
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Rows: uint16(cfg.H), Cols: uint16(cfg.W)})
	if err != nil {
		return nil, err
	}
	s := &Session{
		emu:      vt.NewSafeEmulator(cfg.W, cfg.H),
		ptmx:     ptmx,
		cmd:      c,
		title:    filepath.Base(dir),
		id:       cfg.ID,
		onChange: onChange,
		wsig:     make(chan struct{}, 1),
		wdone:    make(chan struct{}),
	}
	s.emu.SetCallbacks(vt.Callbacks{
		EnableMode:  func(m ansi.Mode) { s.setMode(m, true) },
		DisableMode: func(m ansi.Mode) { s.setMode(m, false) },
	})
	go s.pump()
	go s.writer()
	go s.replies()
	return s, nil
}

// childEnv is base plus the extra KEY=VALUE entries; a bare KEY takes that variable out.
func childEnv(base, extra []string) []string {
	drop := map[string]bool{}
	var add []string
	for _, e := range extra {
		if strings.Contains(e, "=") {
			add = append(add, e)
		} else {
			drop[e] = true
		}
	}
	var out []string
	for _, e := range base {
		if k, _, _ := strings.Cut(e, "="); !drop[k] {
			out = append(out, e)
		}
	}
	return append(out, add...)
}

func (s *Session) changed() {
	if s.onChange != nil {
		s.onChange()
	}
}

func (s *Session) setMode(m ansi.Mode, on bool) {
	switch m {
	case ansi.ModeBracketedPaste:
		s.bracketed.Store(on)
	case ansi.ModeMouseNormal:
		s.setMouse(1, on)
	case ansi.ModeMouseButtonEvent:
		s.setMouse(2, on)
	case ansi.ModeMouseAnyEvent:
		s.setMouse(4, on)
	}
}

func (s *Session) setMouse(bit int32, on bool) {
	if on {
		s.mouse.Or(bit)
	} else {
		s.mouse.And(^bit)
	}
}

// MouseWanted reports whether the child turned mouse tracking on, so mouse events should reach it.
func (s *Session) MouseWanted() bool { return s.mouse.Load() != 0 }

// PID is the process id of the child.
func (s *Session) PID() int { return s.cmd.Process.Pid }

// BracketedPaste reports whether the child enabled bracketed paste.
func (s *Session) BracketedPaste() bool { return s.bracketed.Load() }

// replies forwards what the emulator answers (cursor and device reports) to the child.
func (s *Session) replies() {
	buf := make([]byte, 4096)
	for {
		n, err := s.emu.Read(buf)
		if n > 0 {
			s.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// writer drains queued input into the PTY, so Write never blocks the caller.
func (s *Session) writer() {
	for {
		select {
		case <-s.wsig:
		case <-s.wdone:
			return
		}
		s.wmu.Lock()
		out := s.wbuf
		s.wbuf = nil
		s.wmu.Unlock()
		if len(out) > 0 {
			if _, err := s.ptmx.Write(out); err != nil {
				return
			}
		}
	}
}

// show feeds the emulator; it also runs on the hold's timer, so it recovers by itself.
func (s *Session) show(p []byte) {
	defer func() {
		if recover() != nil { // a parser bug must not take the whole app down
			s.exited.Store(true)
		}
		s.changed()
	}()
	s.emu.Write(p)
}

func (s *Session) pump() {
	hold := &syncHold{emit: s.show}
	buf := make([]byte, 32<<10)
	var osc oscFilter
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			hold.feed(osc.clean(buf[:n]))
		}
		if err != nil {
			break
		}
	}
	hold.flush()
	s.cmd.Wait()
	s.exited.Store(true)
	s.changed()
}

// Write queues p for the child and returns at once, even if the child is not reading.
func (s *Session) Write(p []byte) error {
	s.wmu.Lock()
	s.wbuf = append(s.wbuf, p...)
	s.wmu.Unlock()
	select {
	case s.wsig <- struct{}{}:
	default:
	}
	return nil
}

func (s *Session) Resize(w, h int) {
	if w < 1 || h < 1 {
		return
	}
	s.emu.Resize(w, h)
	pty.Setsize(s.ptmx, &pty.Winsize{Rows: uint16(h), Cols: uint16(w)})
}

func (s *Session) Exited() bool { return s.exited.Load() }

func (s *Session) Draw(scr uv.Screen, area uv.Rectangle) { s.emu.Draw(scr, area) }

func (s *Session) Cursor() (x, y int, hidden bool) {
	p := s.emu.CursorPosition()
	return p.X, p.Y, s.emu.CursorHidden()
}

// Text returns the screen as plain text.
func (s *Session) Text() string {
	lines := strings.Split(ansi.Strip(s.emu.Render()), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

func (s *Session) Title() string { return s.title }

func (s *Session) ID() string { return s.id }
