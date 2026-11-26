package main

import (
	"flag"
	"io"
	"testing"

	"github.com/kalyskhairy/nakama/internal/app"
)

func TestNotifyFlagPicksTheKinds(t *testing.T) {
	all := []app.NoticeKind{app.NoticeWaiting, app.NoticeFinished, app.NoticeContext}
	for args, want := range map[string][3]bool{
		"":                           {true, true, true},
		"--notify":                   {true, true, true},
		"--notify=true":              {true, true, true},
		"--notify=all":               {true, true, true},
		"--notify=false":             {false, false, false},
		"--notify=none":              {false, false, false},
		"--notify=waiting":           {true, false, false},
		"--notify=waiting,context":   {true, false, true},
		"--notify=finished, waiting": {true, true, false},
	} {
		f := allNotices()
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		fs.Var(f, "notify", "")
		var in []string
		if args != "" {
			in = []string{args}
		}
		if err := fs.Parse(in); err != nil {
			t.Fatalf("%q: %v", args, err)
		}
		for i, k := range all {
			if f.on[k] != want[i] {
				t.Errorf("%q: %s = %v", args, k, f.on[k])
			}
		}
	}
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Var(allNotices(), "notify", "")
	if fs.Parse([]string{"--notify=wating"}) == nil {
		t.Error("a typo must be refused")
	}
}
