package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/kalyskhairy/nakama/internal/index"
	"github.com/kalyskhairy/nakama/internal/prefs"
)

// FilterPicker chooses which folders the tree shows.
type FilterPicker struct {
	items  []filterItem
	cursor int
	saved  bool
}

type filterItem struct {
	cwd   string
	count int
	on    bool
}

func NewFilterPicker(folders []index.Folder, f *prefs.Folders) *FilterPicker {
	p := &FilterPicker{}
	for _, fo := range folders {
		p.items = append(p.items, filterItem{fo.Cwd, fo.Count, f.Shown(fo.Cwd)})
	}
	return p
}

// Key handles one key and reports whether the picker is finished.
func (p *FilterPicker) Key(k string) bool {
	switch k {
	case "up", "k":
		p.cursor = max(p.cursor-1, 0)
	case "down", "j":
		p.cursor = min(p.cursor+1, max(len(p.items)-1, 0))
	case " ", "space":
		if p.cursor < len(p.items) {
			p.items[p.cursor].on = !p.items[p.cursor].on
		}
	case "a":
		for i := range p.items {
			p.items[i].on = true
		}
	case "enter":
		checked := false
		for _, it := range p.items {
			checked = checked || it.on
		}
		if !checked && p.cursor < len(p.items) { // Enter on a highlighted folder reads as "this one"
			p.items[p.cursor].on = true
		}
		p.saved = true
		return true
	case "esc":
		return true
	}
	return false
}

// Saved reports whether the picker was finished with Enter.
func (p *FilterPicker) Saved() bool { return p.saved }

// Apply stores the choice, but only if the picker was finished with Enter.
func (p *FilterPicker) Apply(f *prefs.Folders) {
	if !p.saved {
		return
	}
	for _, it := range p.items {
		f.Set(it.cwd, it.on)
	}
}

func (p *FilterPicker) Lines(w, h int) []string {
	out := []string{"Folders shown", "Space check · a all · Enter save (nothing checked: the highlighted one) · esc cancel", ""}
	vis := max(h-len(out), 1)
	off := ScrollOffset(p.cursor, vis)
	for i := off; i < len(p.items) && i < off+vis; i++ {
		it := p.items[i]
		box, cur := "[ ]", "  "
		if it.on {
			box = "[x]"
		}
		if i == p.cursor {
			cur = "> "
		}
		out = append(out, fmt.Sprintf("%s%s %s (%d)  %s", cur, box, index.FolderName(it.cwd), it.count, it.cwd))
	}
	return out
}

// NewPicker chooses the folder and account of a new session.
type NewPicker struct {
	Query          string
	folders        []index.Folder
	accounts       []string
	defaultAccount func(cwd string) string
	cursor         int
	acct           string   // chosen with Tab; empty means the default for the folder
	choices        []string // folders the last Tab could not choose between
	finished, ok   bool
}

func NewNewPicker(folders []index.Folder, accounts []string, defaultAccount func(cwd string) string) *NewPicker {
	return &NewPicker{folders: folders, accounts: accounts, defaultAccount: defaultAccount}
}

func (p *NewPicker) visible() []index.Folder {
	q := strings.ToLower(p.Query)
	var out []index.Folder
	for _, f := range p.folders {
		if f.Cwd != "" && (q == "" || strings.Contains(strings.ToLower(f.Cwd), q)) {
			out = append(out, f)
		}
	}
	return out
}

func (p *NewPicker) isPath() bool {
	return strings.HasPrefix(p.Query, "/") || strings.HasPrefix(p.Query, "~")
}

func (p *NewPicker) dir() string {
	if p.isPath() {
		return p.Query
	}
	if v := p.visible(); len(v) > 0 {
		return v[min(p.cursor, len(v)-1)].Cwd
	}
	return ""
}

func (p *NewPicker) account(dir string) string {
	if p.acct != "" {
		return p.acct
	}
	if p.defaultAccount != nil {
		if a := p.defaultAccount(dir); a != "" {
			return a
		}
	}
	if len(p.accounts) > 0 {
		return p.accounts[0]
	}
	return "personal"
}

func (p *NewPicker) nextAccount() {
	cur := p.account(p.dir())
	for i, a := range p.accounts {
		if a == cur {
			p.acct = p.accounts[(i+1)%len(p.accounts)]
			return
		}
	}
	if len(p.accounts) > 0 {
		p.acct = p.accounts[0]
	}
}

// complete lengthens a typed path the way a shell does, with folders only: one match is filled in
// with its "/", several are filled in as far as they agree, and a second Tab lists them.
func (p *NewPicker) complete() {
	if p.Query == "~" {
		p.Query = "~/"
		return
	}
	cut := strings.LastIndex(p.Query, "/") + 1
	dir, prefix := p.Query[:cut], p.Query[cut:]
	base := dir
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		base = filepath.Join(home, dir[2:])
	}
	entries, err := os.ReadDir(base)
	if dir == "" || err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || (strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".")) {
			continue
		}
		if st, err := os.Stat(filepath.Join(base, name)); err == nil && st.IsDir() { // follows links
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return
	}
	common := names[0]
	for _, n := range names[1:] {
		for !strings.HasPrefix(n, common) {
			common = common[:len(common)-1]
		}
	}
	switch {
	case len(names) == 1:
		p.Query = dir + names[0] + "/"
	case len(common) > len(prefix):
		p.Query = dir + common
	default:
		p.choices = names
	}
}

func (p *NewPicker) Key(k string) bool {
	p.choices = nil
	switch k {
	case "up":
		p.cursor = max(p.cursor-1, 0)
	case "down":
		p.cursor = min(p.cursor+1, max(len(p.visible())-1, 0))
	case "tab":
		if p.isPath() {
			p.complete()
		} else {
			p.nextAccount()
		}
	case "shift-tab":
		p.nextAccount()
	case "backspace":
		if _, n := utf8.DecodeLastRuneInString(p.Query); n > 0 {
			p.Query = p.Query[:len(p.Query)-n]
			p.cursor = 0
		}
	case "enter":
		p.finished, p.ok = true, p.dir() != ""
		return true
	case "esc":
		p.finished = true
		return true
	default:
		if utf8.RuneCountInString(k) == 1 && k >= " " {
			p.Query += k
			p.cursor = 0
		}
	}
	return false
}

// Result is the folder and account chosen; ok is false when the picker was cancelled or nothing was chosen.
func (p *NewPicker) Result() (dir, account string, ok bool) {
	if !p.finished || !p.ok {
		return "", "", false
	}
	d := p.dir()
	return d, p.account(d), true
}

func (p *NewPicker) Lines(w, h int) []string {
	hint := "Type to filter folders, or write a path (/ or ~) · Tab account · Enter start · esc cancel"
	if p.isPath() {
		hint = "Tab completes the path · Shift+Tab account · Enter start · esc cancel"
	}
	out := []string{"New session", hint, "> " + p.Query, "Account: " + p.account(p.dir()), ""}
	vis := max(h-len(out), 1)
	if len(p.choices) > 0 {
		for _, c := range p.choices[:min(len(p.choices), vis)] {
			out = append(out, "  "+c+"/")
		}
		return out
	}
	v := p.visible()
	off := ScrollOffset(p.cursor, vis)
	for i := off; i < len(v) && i < off+vis; i++ {
		cur := "  "
		if i == p.cursor && !p.isPath() {
			cur = "> "
		}
		out = append(out, fmt.Sprintf("%s%s  %s", cur, index.FolderName(v[i].Cwd), v[i].Cwd))
	}
	return out
}
