// Package live finds the running `claude` processes from the session records Claude Code keeps
// in <config dir>/sessions/<pid>.json.
package live

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

type Proc struct {
	PID                                     int
	SessionID, Cwd, Name, Status, ConfigDir string
}

var isAlive = procAlive

// procAlive reports whether pid is a running claude. The cmdline check guards against a reused pid.
func procAlive(pid int) bool {
	if pid <= 0 || syscall.Kill(pid, 0) != nil {
		return false
	}
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	return err == nil && bytes.Contains(b, []byte("claude"))
}

// Read lists the live claude processes recorded in each config dir. Broken records are skipped.
func Read(configDirs []string) []Proc {
	var out []Proc
	for _, dir := range configDirs {
		files, _ := filepath.Glob(filepath.Join(dir, "sessions", "*.json"))
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			var r struct {
				PID       int    `json:"pid"`
				SessionID string `json:"sessionId"`
				Cwd       string `json:"cwd"`
				Name      string `json:"name"`
				Status    string `json:"status"`
			}
			if json.Unmarshal(b, &r) != nil || r.PID <= 0 || r.SessionID == "" || !isAlive(r.PID) {
				continue
			}
			out = append(out, Proc{PID: r.PID, SessionID: r.SessionID, Cwd: r.Cwd, Name: r.Name, Status: r.Status, ConfigDir: dir})
		}
	}
	return out
}

func BySession(ps []Proc) map[string]Proc {
	m := make(map[string]Proc, len(ps))
	for _, p := range ps {
		m[p.SessionID] = p
	}
	return m
}

func ByPID(ps []Proc, pid int) (Proc, bool) {
	for _, p := range ps {
		if p.PID == pid {
			return p, true
		}
	}
	return Proc{}, false
}
