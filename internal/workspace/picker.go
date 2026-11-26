package workspace

import (
	"fmt"
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
	out := []string{"Folder yang ditampilkan", "Space centang · a semua · Enter simpan (tanpa centang: folder yang disorot) · esc batal", ""}
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
	acct           string // chosen with Tab; empty means the default for the folder
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

func (p *NewPicker) Key(k string) bool {
	switch k {
	case "up":
		p.cursor = max(p.cursor-1, 0)
	case "down":
		p.cursor = min(p.cursor+1, max(len(p.visible())-1, 0))
	case "tab":
		cur := p.account(p.dir())
		for i, a := range p.accounts {
			if a == cur {
				p.acct = p.accounts[(i+1)%len(p.accounts)]
				return false
			}
		}
		if len(p.accounts) > 0 {
			p.acct = p.accounts[0]
		}
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
	out := []string{"Sesi baru", "Ketik untuk menyaring folder, atau tulis path (/ atau ~) · Tab ganti akun · Enter mulai · esc batal", "> " + p.Query,
		"Akun: " + p.account(p.dir()), ""}
	vis := max(h-len(out), 1)
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
