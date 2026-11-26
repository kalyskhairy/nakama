package theme

import "testing"

func TestLoadCastAndHeights(t *testing.T) {
	th, err := Load(write(t, func(m map[string]any) {
		m["sessionCharacters"] = []string{"captain"}
		m["heights"] = map[string]any{"captain": 0.7}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(th.SessionCharacters) != 1 || th.Heights["captain"] != 0.7 {
		t.Fatalf("%v %v", th.SessionCharacters, th.Heights)
	}
	for name, mut := range map[string]func(map[string]any){
		"unknown in cast":    func(m map[string]any) { m["sessionCharacters"] = []string{"ghost"} },
		"unknown in heights": func(m map[string]any) { m["heights"] = map[string]any{"ghost": 1} },
		"zero height":        func(m map[string]any) { m["heights"] = map[string]any{"captain": 0} },
	} {
		if _, err := Load(write(t, mut)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestCorridorNeedsExistingNodes(t *testing.T) {
	th, err := Load(write(t, func(m map[string]any) { m["corridor"] = []string{"a", "b"} }))
	if err != nil || len(th.Corridor) != 2 {
		t.Fatalf("%v", err)
	}
	for _, bad := range [][]string{{"a"}, {"a", "nope"}} {
		if _, err := Load(write(t, func(m map[string]any) { m["corridor"] = bad })); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}
