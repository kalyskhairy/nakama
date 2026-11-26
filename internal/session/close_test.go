package session

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"testing"
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
