package main

import (
	"fmt"
	"strings"

	"github.com/kalyskhairy/nakama/internal/app"
)

// noticeFlag is --notify: all (also bare or true), none (or false), or a list of kinds.
type noticeFlag struct{ on map[app.NoticeKind]bool }

var noticeKinds = []app.NoticeKind{app.NoticeWaiting, app.NoticeFinished, app.NoticeContext}

func allNotices() *noticeFlag {
	f := &noticeFlag{on: map[app.NoticeKind]bool{}}
	for _, k := range noticeKinds {
		f.on[k] = true
	}
	return f
}

func (f *noticeFlag) String() string   { return "all" }
func (f *noticeFlag) IsBoolFlag() bool { return true } // a bare --notify stays valid

func (f *noticeFlag) Set(v string) error {
	on := map[app.NoticeKind]bool{}
	switch v {
	case "all", "true":
		on = allNotices().on
	case "none", "false":
	default:
		for _, name := range strings.Split(v, ",") {
			k := app.NoticeKind(strings.TrimSpace(name))
			if k != app.NoticeWaiting && k != app.NoticeFinished && k != app.NoticeContext {
				return fmt.Errorf("unknown notification %q (waiting, finished, context, all, none)", name)
			}
			on[k] = true
		}
	}
	f.on = on
	return nil
}
