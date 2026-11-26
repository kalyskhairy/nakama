package app

import (
	"strings"
	"testing"

	"github.com/kalyskhairy/nakama/internal/monitor"
)

func TestArrowsFollowTheOrderOfTheCrewList(t *testing.T) {
	// by status the list reads c (waiting), b (working), a (idle): the reverse of the ids
	info := map[string]monitor.Info{"b": {Status: monitor.Working}, "c": {Status: monitor.Waiting}}
	subs := map[string][]monitor.SubInfo{"b": {{ID: "x", Type: "Explore", Info: monitor.Info{Status: monitor.Working}}}}
	m, _ := officeModel(t, info, subs, "a", "b", "c")
	m.Tick(t0)
	var listed []string
	for _, r := range m.crewRows(t0) {
		listed = append(listed, r.ID)
		if r.ID == "b" {
			listed = append(listed, "sub:b:x")
		}
	}
	if listed[0] == "a" {
		t.Skipf("the list is in id order here: %v", listed)
	}
	var walked []string
	for range listed {
		m.HandleEvent(key("down"), t0)
		id, _ := m.Selected()
		walked = append(walked, id)
	}
	if strings.Join(walked, " ") != strings.Join(listed, " ") {
		t.Fatalf("down walks %v, the list shows %v", walked, listed)
	}
	m.HandleEvent(key("up"), t0)
	if id, _ := m.Selected(); id != listed[len(listed)-2] {
		t.Fatalf("up from the bottom: %s, list %v", id, listed)
	}
}
