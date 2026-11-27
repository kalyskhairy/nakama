package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
)

// alive reports a process that still runs (a zombie does not).
func alive(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	return err == nil && syscall.Kill(pid, 0) == nil && !regexp.MustCompile(`\) Z `).Match(b)
}

func TestCloseKillsWhatTheChildStarted(t *testing.T) {
	// the child ignores the hang-up and starts a job in its own process group, as tools do
	s := start(t, nil, "bash", "-c", `trap "" HUP; set -m; sleep 60 & echo job=$!; wait`)
	var job int
	waitFor(t, "job pid", func() bool {
		if m := regexp.MustCompile(`job=(\d+)`).FindStringSubmatch(s.Text()); m != nil {
			job, _ = strconv.Atoi(m[1])
		}
		return job > 0
	})
	child := s.PID()
	s.Close()
	waitFor(t, "child gone", func() bool { return !alive(child) })
	waitFor(t, "job gone", func() bool { return !alive(job) })
}

func TestCloseLetsTheChildFinishItsFiles(t *testing.T) {
	out := filepath.Join(t.TempDir(), "saved")
	s := start(t, nil, "sh", "-c", `trap 'echo ok > `+out+`; exit 0' HUP; echo ready; while :; do sleep 0.05; done`)
	waitFor(t, "ready", func() bool { return len(s.Text()) > 0 && regexp.MustCompile("ready").MatchString(s.Text()) })
	s.Close()
	waitFor(t, "saved on hang-up", func() bool { _, err := os.Stat(out); return err == nil })
}

func TestCloseFreesTheEmulator(t *testing.T) {
	s := start(t, nil, "sh", "-c", "sleep 30")
	s.Close()
	done := make(chan struct{})
	go func() { s.emu.Read(make([]byte, 8)); close(done) }() // what the replies goroutine waits on
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the emulator is still open: its reader waits forever and keeps the screen in memory")
	}
}

func TestCloseNeverTouchesAProcessThatTookTheSameNumber(t *testing.T) {
	// a program of its own, in its own terminal session, standing in for one that reused our number
	other := func() *exec.Cmd {
		c := exec.Command("sleep", "60")
		c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Process.Kill(); c.Wait() })
		return c
	}
	session := func(c *exec.Cmd, start string) *Session {
		return &Session{cmd: c, start: start, wdone: make(chan struct{}), emu: vt.NewSafeEmulator(10, 5)}
	}
	c := other()
	session(c, startTime(c.Process.Pid)+"0").Close()
	if !alive(c.Process.Pid) {
		t.Fatal("killed a process that is not ours")
	}
	c = other()
	session(c, startTime(c.Process.Pid)).Close()
	waitFor(t, "our own process ended", func() bool { return !alive(c.Process.Pid) })
}
