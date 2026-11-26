package monitor

import (
	"os"
	"path/filepath"
)

// Roots returns the projects directories of every Claude Code account:
// ~/.claude/projects and ~/.claude-*/projects, where they exist.
func Roots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	cands, _ := filepath.Glob(filepath.Join(home, ".claude-*", "projects"))
	cands = append([]string{filepath.Join(home, ".claude", "projects")}, cands...)
	var out []string
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			out = append(out, c)
		}
	}
	return out
}

// Locate finds <root>/<any project dir>/<sessionID>.jsonl, or "" if it does not exist yet.
// The project directory name is an encoding of the working directory, so it is globbed, not computed.
func Locate(roots []string, sessionID string) string {
	if sessionID == "" {
		return ""
	}
	for _, r := range roots {
		if m, _ := filepath.Glob(filepath.Join(r, "*", sessionID+".jsonl")); len(m) > 0 {
			return m[0]
		}
	}
	return ""
}
