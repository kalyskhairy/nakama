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

	kind, job, parked string // a background job, and the job a session handed its work to
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
				Kind      string `json:"kind"`
				JobID     string `json:"jobId"`
				Parked    string `json:"parkedJobId"`
			}
			if json.Unmarshal(b, &r) != nil || r.PID <= 0 || r.SessionID == "" || !isAlive(r.PID) {
				continue
			}
			out = append(out, Proc{PID: r.PID, SessionID: r.SessionID, Cwd: r.Cwd, Name: r.Name, Status: r.Status, ConfigDir: dir,
				kind: r.Kind, job: r.JobID, parked: r.Parked})
		}
	}
	return seatJobs(out)
}

// seatJobs folds a background job into the session that handed its work to it (parkedJobId names the
// job): the conversation goes on under the job's session id and status, in the terminal of the
// session that parked it. Seen as two, the job would look like a session in another terminal.
func seatJobs(ps []Proc) []Proc {
	jobs := map[string]Proc{}
	for _, p := range ps {
		if p.kind == "bg" && p.job != "" {
			jobs[p.job] = p
		}
	}
	taken := map[int]bool{}
	for i, p := range ps {
		if j, ok := jobs[p.parked]; ok && p.kind != "bg" {
			ps[i].SessionID, ps[i].Status = j.SessionID, j.Status
			taken[j.PID] = true
		}
	}
	out := ps[:0]
	for _, p := range ps {
		if !taken[p.PID] || p.kind != "bg" {
			out = append(out, p)
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
