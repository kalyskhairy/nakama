package session

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"testing"
	"time"
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
	s := start(t, nil, "sh", "-c", "sleep 60")
	pid := s.PID()
	s.start += "0" // as if the child had ended and another program now had its number
	s.Close()
	if !alive(pid) {
		t.Fatal("killed a process that is not ours")
	}
	syscall.Kill(pid, syscall.SIGKILL)
}
