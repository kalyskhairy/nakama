package session

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func start(t *testing.T, onChange func(), name string, args ...string) *Session {
	t.Helper()
	s, err := Start(Config{Cmd: name, Args: args, W: 80, H: 24}, onChange)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestOutputReachesEmulator(t *testing.T) {
	s := start(t, nil, "sh", "-c", "printf hello; sleep 5")
	waitFor(t, "hello on screen", func() bool { return strings.Contains(s.Text(), "hello") })
}

func TestInputReachesChild(t *testing.T) {
	s := start(t, nil, "cat")
	if err := s.Write([]byte("abc\r")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "abc echoed", func() bool { return strings.Contains(s.Text(), "abc") })
}

func TestResizeReachesChild(t *testing.T) {
	s := start(t, nil, "sh", "-c", "sleep 0.4; stty size; sleep 5")
	s.Resize(50, 20)
	waitFor(t, "stty size 20 50", func() bool { return strings.Contains(s.Text(), "20 50") })
}

func TestEmulatorRepliesReachChild(t *testing.T) {
	s := start(t, nil, "bash", "-c",
		`printf '\033[6n'; IFS= read -rs -d R r; printf 'GOT:%s' "$(printf %s "$r" | cat -v)"; sleep 5`)
	waitFor(t, "cursor report reaches child", func() bool { return strings.Contains(s.Text(), "GOT:^[[1;1") })
}

func TestExitedAfterChildEnds(t *testing.T) {
	var calls atomic.Int32
	s := start(t, func() { calls.Add(1) }, "sh", "-c", "exit 0")
	waitFor(t, "Exited()", s.Exited)
	if calls.Load() == 0 {
		t.Fatal("onChange never called")
	}
}

func TestStartFailsForMissingCommand(t *testing.T) {
	if _, err := Start(Config{Cmd: "tidak-ada-perintah-ini", W: 80, H: 24}, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestCloseKillsChildAndIsIdempotent(t *testing.T) {
	s := start(t, nil, "sleep", "30")
	s.Close()
	s.Close()
	waitFor(t, "Exited() after Close", s.Exited)
}

func TestTitleIsDirBase(t *testing.T) {
	s, err := Start(Config{Cmd: "sleep", Args: []string{"5"}, Dir: "/tmp", W: 80, H: 24}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := s.Title(); got != "tmp" {
		t.Fatalf("Title() = %q", got)
	}
}

func TestWriteNeverBlocks(t *testing.T) {
	s := start(t, nil, "sh", "-c", "stty raw -echo; sleep 30") // child never reads stdin
	time.Sleep(200 * time.Millisecond)
	done := make(chan struct{})
	go func() { s.Write(make([]byte, 1<<20)); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Write blocked on a child that does not read")
	}
}

func TestBracketedPasteTracksChildMode(t *testing.T) {
	s := start(t, nil, "sh", "-c", `printf '\033[?2004h'; sleep 30`)
	waitFor(t, "mode 2004", s.BracketedPaste)
	plain := start(t, nil, "sleep", "30")
	if plain.BracketedPaste() {
		t.Fatal("plain child should not have bracketed paste")
	}
}

func TestSessionKeepsID(t *testing.T) {
	s, err := Start(Config{Cmd: "sleep", Args: []string{"5"}, W: 80, H: 24, ID: "u-1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.ID() != "u-1" {
		t.Fatalf("ID() = %q", s.ID())
	}
}

func TestStartPassesEnv(t *testing.T) {
	s, err := Start(Config{Cmd: "sh", Args: []string{"-c", "printf got-$CREWDECK_T; sleep 5"}, Env: []string{"CREWDECK_T=bar"}, W: 80, H: 24}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	waitFor(t, "env on screen", func() bool { return strings.Contains(s.Text(), "got-bar") })
}

func TestPIDIsChildPID(t *testing.T) {
	s := start(t, nil, "sh", "-c", "printf $$; sleep 5")
	waitFor(t, "pid printed", func() bool { return strings.Contains(s.Text(), fmt.Sprint(s.PID())) })
	if s.PID() <= 0 {
		t.Fatal(s.PID())
	}
}

func TestMouseWantedTracksChildMode(t *testing.T) {
	s := start(t, nil, "sh", "-c", `printf '\033[?1000h\033[?1006h'; sleep 0.5; printf '\033[?1000l'; sleep 5`)
	waitFor(t, "mouse on", s.MouseWanted)
	waitFor(t, "mouse off", func() bool { return !s.MouseWanted() })
}
