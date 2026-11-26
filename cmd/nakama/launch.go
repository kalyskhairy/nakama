package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kalyskhairy/nakama/internal/app"
)

// launchArgs says how to start claude: a new session gets an id of our choosing, so its transcript
// can be found without guessing; a resumed one is resumed by id. An account other than the default
// needs its config dir in the environment, and the default one
// needs it out. Other commands (tests, other programs) get nothing extra.
func launchArgs(cmd string, r app.SpawnReq) (args []string, id string, env []string) {
	if filepath.Base(cmd) != "claude" {
		return nil, "", nil
	}
	if r.ConfigDir != "" {
		if home, err := os.UserHomeDir(); err != nil || r.ConfigDir != filepath.Join(home, ".claude") {
			env = []string{"CLAUDE_CONFIG_DIR=" + r.ConfigDir}
		} else {
			env = []string{"CLAUDE_CONFIG_DIR"} // the default account: not whatever nakama inherited
		}
	}
	if r.ResumeID != "" {
		return []string{"--resume", r.ResumeID}, r.ResumeID, env
	}
	id = newUUID()
	return []string{"--session-id", id}, id, env
}

func newUUID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40 // version 4
	b[8] = b[8]&0x3f | 0x80 // variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
