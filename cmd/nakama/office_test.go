package main

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kalyskhairy/nakama/internal/sprites"
)

// writeTheme creates a minimal valid theme called name under themesDir.
func writeTheme(t *testing.T, themesDir, name string) {
	t.Helper()
	dir := filepath.Join(themesDir, name)
	os.MkdirAll(dir, 0o755)
	bg := image.NewNRGBA(image.Rect(0, 0, 100, 50))
	for i := range bg.Pix {
		bg.Pix[i] = 120
	}
	f, _ := os.Create(filepath.Join(dir, "bg.png"))
	png.Encode(f, bg)
	f.Close()
	s := &sprites.Sheet{}
	box := func(w, h int) *image.NRGBA {
		im := image.NewNRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				im.SetNRGBA(x, y, color.NRGBA{200, 40, 40, 255})
			}
		}
		return im
	}
	for ri := 0; ri < 6; ri++ {
		s.Rows[ri] = []*image.NRGBA{box(8, 16)}
	}
	if err := s.Save(filepath.Join(dir, "sprites", "hero")); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{
		"name": name, "background": map[string]any{"file": "bg.png"}, "charHeight": 0.3,
		"characters": map[string]any{"hero": "sprites/hero"}, "agentTypes": map[string]any{"session": "hero", "default": "hero"},
		"nodes": map[string]any{"a": map[string]any{"x": 0.1, "y": 0.5}}, "edges": [][]string{}, "entrance": "a",
		"stations": []any{
			map[string]any{"id": "w", "x": 0.5, "y": 0.5, "kind": "work", "node": "a"},
			map[string]any{"id": "r", "x": 0.3, "y": 0.5, "kind": "rest", "node": "a"},
		},
	})
	os.WriteFile(filepath.Join(dir, "theme.json"), b, 0o644)
}

func TestLoadOfficeReasons(t *testing.T) {
	themes := t.TempDir()
	writeTheme(t, themes, "good")
	yes, no := func() bool { return true }, func() bool { return false }
	o := officeOpts{themesDir: themes, graphics: "sixel", fd: -1, maxScale: 0.5}

	cases := []struct {
		name   string
		mutate func(*officeOpts)
		detect func() bool
		want   string // substring of Reason; "" means the office must load
	}{
		{"valid theme", func(o *officeOpts) { o.themeName = "good" }, yes, ""},
		{"missing theme", func(o *officeOpts) { o.themeName = "nope" }, yes, "nope"},
		{"graphics off", func(o *officeOpts) { o.themeName = "good"; o.graphics = "off" }, yes, "graphics"},
		{"auto without sixel", func(o *officeOpts) { o.themeName = "good"; o.graphics = "auto" }, no, "sixel"},
		{"auto with sixel", func(o *officeOpts) { o.themeName = "good"; o.graphics = "auto" }, yes, ""},
	}
	for _, c := range cases {
		oo := o
		c.mutate(&oo)
		cfg := loadOffice(oo, c.detect)
		if c.want == "" {
			if cfg.Reason != "" || cfg.Renderer == nil || cfg.Theme == nil || cfg.CellW <= 0 || cfg.CellH <= 0 || cfg.MaxScale != 0.5 {
				t.Errorf("%s: %+v", c.name, cfg)
			}
		} else if !strings.Contains(cfg.Reason, c.want) {
			t.Errorf("%s: reason %q does not mention %q", c.name, cfg.Reason, c.want)
		}
	}

	broken := t.TempDir()
	os.MkdirAll(filepath.Join(broken, "bad"), 0o755)
	os.WriteFile(filepath.Join(broken, "bad", "theme.json"), []byte("{nope"), 0o644)
	oo := o
	oo.themesDir, oo.themeName = broken, "bad"
	if cfg := loadOffice(oo, yes); !strings.Contains(cfg.Reason, "json") {
		t.Errorf("broken theme: %q", cfg.Reason)
	}
}

func TestDefaultThemesDirHonoursXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/x/cfg")
	if got := defaultThemesDir(); got != "/x/cfg/nakama/themes" {
		t.Fatalf("got %q", got)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/h")
	if got := defaultThemesDir(); got != "/h/.config/nakama/themes" {
		t.Fatalf("got %q", got)
	}
}

func TestWithoutADefaultThemeTheDemoIsUsed(t *testing.T) {
	dir := t.TempDir()
	cfg := loadOffice(officeOpts{themeName: "default", themesDir: dir, graphics: "sixel"}, func() bool { return true })
	if cfg.Reason != "" || cfg.Theme == nil || cfg.Theme.Name != "demo" {
		t.Fatalf("%q", cfg.Reason)
	}
	// a name the owner asked for is never replaced
	if cfg := loadOffice(officeOpts{themeName: "mine", themesDir: dir, graphics: "sixel"}, func() bool { return true }); cfg.Reason == "" {
		t.Fatal("a missing named theme must be reported")
	}
}
