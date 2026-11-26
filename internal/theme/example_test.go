package theme

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// The example in the README must stay a theme that loads.
func TestExampleThemeLoads(t *testing.T) {
	b, err := os.ReadFile("../../examples/theme.json")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, d := range []string{"sprites/captain", "sprites/cook"} {
		os.MkdirAll(filepath.Join(dir, d), 0o755)
	}
	os.WriteFile(filepath.Join(dir, "theme.json"), b, 0o644)
	f, _ := os.Create(filepath.Join(dir, "background.png"))
	png.Encode(f, image.NewNRGBA(image.Rect(0, 0, 40, 20)))
	f.Close()
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
}
