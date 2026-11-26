package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/kalyskhairy/nakama/internal/app"
)

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestLaunchArgsNewSession(t *testing.T) {
	for _, cmd := range []string{"claude", "/usr/bin/claude", "/home/user/.local/bin/claude"} {
		args, id, env := launchArgs(cmd, app.SpawnReq{Dir: "/p"})
		if !uuidRe.MatchString(id) || len(args) != 2 || args[0] != "--session-id" || args[1] != id || env != nil {
			t.Errorf("%s: args=%v id=%q env=%v", cmd, args, id, env)
		}
	}
	_, id1, _ := launchArgs("claude", app.SpawnReq{})
	_, id2, _ := launchArgs("claude", app.SpawnReq{})
	if id1 == id2 {
		t.Error("ids must be unique")
	}
}

func TestLaunchArgsResume(t *testing.T) {
	args, id, _ := launchArgs("claude", app.SpawnReq{Dir: "/p", ResumeID: "abc-123"})
	if id != "abc-123" || len(args) != 2 || args[0] != "--resume" || args[1] != "abc-123" {
		t.Fatalf("args=%v id=%q", args, id)
	}
}

func TestLaunchArgsNonClaudeGetsNothing(t *testing.T) {
	args, id, env := launchArgs("sh", app.SpawnReq{Dir: "/p", ResumeID: "x", ConfigDir: "/h/.claude-work"})
	if args != nil || id != "" || env != nil {
		t.Fatalf("args=%v id=%q env=%v", args, id, env)
	}
}

func TestLaunchEnvForNonDefaultAccountOnly(t *testing.T) {
	home, _ := os.UserHomeDir()
	if _, _, env := launchArgs("claude", app.SpawnReq{ConfigDir: filepath.Join(home, ".claude")}); len(env) != 1 || env[0] != "CLAUDE_CONFIG_DIR" {
		t.Fatalf("the default account only drops an inherited one: %v", env)
	}
	if _, _, env := launchArgs("claude", app.SpawnReq{ConfigDir: ""}); env != nil {
		t.Fatalf("no config dir, no env: %v", env)
	}
	wd := filepath.Join(home, ".claude-work")
	_, _, env := launchArgs("claude", app.SpawnReq{ConfigDir: wd})
	if len(env) != 1 || env[0] != "CLAUDE_CONFIG_DIR="+wd {
		t.Fatalf("%v", env)
	}
}
