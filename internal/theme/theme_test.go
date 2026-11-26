package theme

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validTheme is a minimal theme.json as a map, so each test can break exactly one thing.
func validTheme() map[string]any {
	return map[string]any{
		"name":       "t",
		"background": map[string]any{"file": "bg.png", "crop": []int{0, 10, 200, 80}},
		"charHeight": 0.1,
		"characters": map[string]any{"captain": "sprites/cap"},
		"agentTypes": map[string]any{"session": "captain", "default": "captain"},
		"nodes":      map[string]any{"a": map[string]any{"x": 0.1, "y": 0.5}, "b": map[string]any{"x": 0.9, "y": 0.5}},
		"edges":      [][]string{{"a", "b"}},
		"entrance":   "a",
		"stations": []any{
			map[string]any{"id": "desk", "x": 0.8, "y": 0.4, "kind": "work", "room": "r1", "for": []string{"session"}, "node": "b"},
			map[string]any{"id": "sofa", "x": 0.2, "y": 0.4, "kind": "rest", "room": "r2", "node": "a"},
		},
	}
}

func write(t *testing.T, mutate func(m map[string]any), skip ...string) string {
	t.Helper()
	dir := t.TempDir()
	skipped := strings.Join(skip, ",")
	if !strings.Contains(skipped, "bg") {
		im := image.NewNRGBA(image.Rect(0, 0, 200, 100))
		for y := 0; y < 100; y++ {
			for x := 0; x < 200; x++ {
				im.Set(x, y, color.NRGBA{90, 90, 90, 255})
			}
		}
		f, _ := os.Create(filepath.Join(dir, "bg.png"))
		png.Encode(f, im)
		f.Close()
	}
	if !strings.Contains(skipped, "sprites") {
		os.MkdirAll(filepath.Join(dir, "sprites", "cap"), 0o755)
	}
	m := validTheme()
	if mutate != nil {
		mutate(m)
	}
	b, _ := json.Marshal(m)
	if !strings.Contains(skipped, "json") {
		os.WriteFile(filepath.Join(dir, "theme.json"), b, 0o644)
	}
	return dir
}

func TestLoadValidTheme(t *testing.T) {
	th, err := Load(write(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if b := th.Background.Bounds(); b.Dx() != 200 || b.Dy() != 80 {
		t.Fatalf("background bounds %v", b)
	}
	if th.Character("session") != "captain" || th.Character("Explore") != "captain" {
		t.Fatalf("character mapping: %q %q", th.Character("session"), th.Character("Explore"))
	}
	if th.CharHeight != 0.1 || th.Entrance != "a" || len(th.Stations) != 2 || len(th.Nodes) != 2 {
		t.Fatalf("%+v", th)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := []struct {
		name, want string
		dir        func() string
	}{
		{"missing theme.json", "theme.json", func() string { return write(t, nil, "json") }},
		{"broken json", "json", func() string {
			d := write(t, nil)
			os.WriteFile(filepath.Join(d, "theme.json"), []byte("{nope"), 0o644)
			return d
		}},
		{"missing background", "background", func() string { return write(t, nil, "bg") }},
		{"crop outside image", "crop", func() string {
			return write(t, func(m map[string]any) { m["background"].(map[string]any)["crop"] = []int{0, 0, 999, 80} })
		}},
		{"station coordinate out of range", "koordinat", func() string {
			return write(t, func(m map[string]any) { m["stations"].([]any)[0].(map[string]any)["x"] = 1.2 })
		}},
		{"station with unknown node", "node", func() string {
			return write(t, func(m map[string]any) { m["stations"].([]any)[0].(map[string]any)["node"] = "zz" })
		}},
		{"no default agent type", "default", func() string {
			return write(t, func(m map[string]any) { m["agentTypes"] = map[string]any{"session": "captain"} })
		}},
		{"unknown character", "karakter", func() string {
			return write(t, func(m map[string]any) { m["agentTypes"].(map[string]any)["Explore"] = "ghost" })
		}},
		{"sprite dir missing", "sprite", func() string { return write(t, nil, "sprites") }},
		{"graph not connected", "terhubung", func() string {
			return write(t, func(m map[string]any) {
				m["nodes"].(map[string]any)["c"] = map[string]any{"x": 0.5, "y": 0.9}
			})
		}},
		{"no rest station", "stasiun", func() string {
			return write(t, func(m map[string]any) { m["stations"] = m["stations"].([]any)[:1] })
		}},
		{"no work station", "stasiun", func() string {
			return write(t, func(m map[string]any) { m["stations"] = m["stations"].([]any)[1:] })
		}},
	}
	for _, c := range cases {
		_, err := Load(c.dir())
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want an error mentioning %q", c.name, err, c.want)
		}
	}
}

func TestPathsAreRelativeToThemeDir(t *testing.T) {
	dir := write(t, nil)
	th, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "sprites", "cap")
	if th.Characters["captain"] != want || th.Dir != dir {
		t.Fatalf("got %q, dir %q", th.Characters["captain"], th.Dir)
	}
}
