package session

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

const hangupWait = time.Second

// signalSession signals every process of the terminal session led by leader: the child and what it
// started, in whatever process group. A daemon that made its own session is left alone.
func signalSession(leader int, sig syscall.Signal) {
	stats, _ := filepath.Glob("/proc/[0-9]*/stat")
	for _, p := range stats {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		f := bytes.Fields(b[bytes.LastIndexByte(b, ')')+1:]) // state ppid pgrp session ...
		if len(f) > 3 && string(f[3]) == strconv.Itoa(leader) {
			if pid, err := strconv.Atoi(filepath.Base(filepath.Dir(p))); err == nil {
				syscall.Kill(pid, sig)
			}
		}
	}
}

// Close ends the session: a hang-up first, so claude can finish writing its transcript, then a kill
// for whatever is left.
func (s *Session) Close() {
	s.closer.Do(func() {
		close(s.wdone)
		if s.cmd.Process != nil {
			pid := s.cmd.Process.Pid
			if !s.Exited() {
				signalSession(pid, syscall.SIGHUP)
				for end := time.Now().Add(hangupWait); !s.Exited() && time.Now().Before(end); {
					time.Sleep(20 * time.Millisecond)
				}
			}
			signalSession(pid, syscall.SIGKILL)
		}
		s.ptmx.Close()
	})
}
