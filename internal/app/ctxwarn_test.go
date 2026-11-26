package app

import (
	"strings"
	"testing"
	"time"

	"github.com/kalyskhairy/nakama/internal/monitor"
)

func TestAFullContextWarnsOnceUntilItIsEmptiedAgain(t *testing.T) {
	m, fm := officeModel(t, map[string]monitor.Info{"a": {CtxPct: 50}}, nil, "a")
	at := func(sec, pct int) []Notice {
		fm.info["a"] = monitor.Info{CtxPct: pct}
		m.Poll(t0.Add(time.Duration(sec) * time.Second))
		return m.TakeNotices()
	}
	at(0, 50)
	if got := at(1, 83); len(got) != 1 || !strings.Contains(got[0].Title, "83%") {
		t.Fatalf("warn: %v", got)
	}
	if got := at(2, 90); len(got) != 0 {
		t.Fatalf("only once: %v", got)
	}
	at(3, 20) // compacted
	if got := at(4, 81); len(got) != 1 {
		t.Fatalf("again after a compaction: %v", got)
	}
	m.Tick(t0.Add(5 * time.Second))
	if a := m.Actors()[0]; !a.Full {
		t.Fatalf("the character must show it: %+v", a)
	}
	if got := strings.Join(m.crewLines(80, 20, t0), "\n"); !strings.Contains(got, "context 81% full") {
		t.Fatalf("the list must say it:\n%s", got)
	}
}
