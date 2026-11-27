// Package demo is the theme nakama shows before the owner has made one: an original guild house and
// crew, AI-generated for this project (see art/).
package demo

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed art
var art embed.FS

// Write puts the demo theme in dir. A theme that is already there is left alone.
func Write(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "theme.json")); err == nil {
		return nil
	}
	files, _ := fs.Sub(art, "art")
	err := fs.WalkDir(files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == "theme.json" { // written last: it marks the theme as complete
			return err
		}
		out := filepath.Join(dir, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := fs.ReadFile(files, p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
	if err != nil {
		return err
	}
	b, err := fs.ReadFile(files, "theme.json")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "theme.json"), b, 0o644)
}
