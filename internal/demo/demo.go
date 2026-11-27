// Package demo is a small office theme shipped with nakama, so the office works before the owner has
// made a theme. Its artwork is original, made with an AI image generator.
package demo

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed theme
var files embed.FS

// version changes whenever the artwork does, so an older copy on disk is replaced.
const version = "2"

// Write puts the demo theme in dir. A copy of this version that is already there is left alone.
func Write(dir string) error {
	if b, err := os.ReadFile(filepath.Join(dir, ".version")); err == nil && string(b) == version {
		if _, err := os.Stat(filepath.Join(dir, "theme.json")); err == nil {
			return nil
		}
	}
	os.RemoveAll(filepath.Join(dir, "sprites")) // an older demo's frames
	err := fs.WalkDir(files, "theme", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || p == "theme/theme.json" {
			return err
		}
		b, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		out := filepath.Join(dir, filepath.FromSlash(p[len("theme/"):]))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
	if err != nil {
		return err
	}
	b, err := files.ReadFile("theme/theme.json")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "theme.json"), b, 0o644); err != nil { // last: marks the theme complete
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".version"), []byte(version), 0o644)
}
