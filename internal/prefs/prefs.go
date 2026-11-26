// Package prefs keeps the owner's choice of which folders to show, in ~/.config/nakama/folders.json.
package prefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

type file struct {
	Shown []string `json:"shown"`
	Seen  []string `json:"seen"`
}

// Folders is the saved folder filter. A folder is new until it has been seen or chosen.
type Folders struct {
	path  string
	first bool
	shown map[string]bool
	seen  map[string]bool
}

func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "nakama", "folders.json")
}

// Load reads the file. A missing or broken file gives an empty filter that counts as a first run.
func Load(path string) *Folders {
	f := &Folders{path: path, first: true, shown: map[string]bool{}, seen: map[string]bool{}}
	b, err := os.ReadFile(path)
	if err != nil {
		return f
	}
	var d file
	if json.Unmarshal(b, &d) != nil {
		return f
	}
	f.first = false
	for _, c := range d.Shown {
		f.shown[c] = true
	}
	for _, c := range d.Seen {
		f.seen[c] = true
	}
	return f
}

func (f *Folders) First() bool           { return f.first }
func (f *Folders) Shown(cwd string) bool { return f.shown[cwd] }
func (f *Folders) IsNew(cwd string) bool { return !f.seen[cwd] }

// Set shows or hides a folder and marks it as seen: the owner has decided about it.
func (f *Folders) Set(cwd string, on bool) {
	f.shown[cwd], f.seen[cwd] = on, true
	if !on {
		delete(f.shown, cwd)
	}
}

func (f *Folders) MarkSeen(cwds []string) {
	for _, c := range cwds {
		f.seen[c] = true
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Save writes the file through a temporary file and a rename, so a crash never leaves half a file.
func (f *Folders) Save() error {
	b, err := json.MarshalIndent(file{Shown: keys(f.shown), Seen: keys(f.seen)}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.path), ".folders-*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), f.path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	f.first = false
	return nil
}
