package index

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	lnCwd   = `{"type":"user","cwd":"D:\\webshop","sessionId":"x","message":{"role":"user","content":"tolong cek halaman toko"}}`
	lnTitle = `{"type":"ai-title","aiTitle":"Kerangka halaman toko","sessionId":"x"}`
	lnAsst  = `{"type":"assistant","message":{"id":"m1","content":[{"type":"text","text":"ok"}]}}`
)

func put(t *testing.T, root, proj, id string, mod time.Time, lines ...string) string {
	t.Helper()
	dir := filepath.Join(root, proj)
	os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(p, mod, mod)
	return p
}

func scan(roots ...string) []Entry {
	return NewIndexer(func() []string { return roots }).Scan()
}

var now = time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)

func TestScanReadsTitleCwdAccount(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".claude-work", "projects")
	put(t, root, "-D-webshop", "id1", now, lnCwd, lnAsst, lnTitle)
	es := scan(root)
	if len(es) != 1 {
		t.Fatalf("%+v", es)
	}
	e := es[0]
	if e.ID != "id1" || e.Account != "work" || e.ConfigDir != filepath.Join(home, ".claude-work") ||
		e.Cwd != `D:\webshop` || e.Title != "Kerangka halaman toko" || !e.ModTime.Equal(now) || e.Size == 0 {
		t.Fatalf("%+v", e)
	}
}

func TestScanNewestFirst(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".claude", "projects")
	put(t, root, "-a", "old", now.Add(-time.Hour), lnCwd)
	put(t, root, "-a", "new", now, lnCwd)
	if es := scan(root); len(es) != 2 || es[0].ID != "new" || es[0].Account != "personal" {
		t.Fatalf("%+v", es)
	}
}

func TestScanIgnoresSubagentFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".claude", "projects")
	put(t, root, "-a", "id1", now, lnCwd)
	sub := filepath.Join(root, "-a", "id1", "subagents")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(sub, "agent-1.jsonl"), []byte(lnCwd+"\n"), 0o644)
	if es := scan(root); len(es) != 1 {
		t.Fatalf("%+v", es)
	}
}

func TestScanUsesCacheUnlessChanged(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".claude", "projects")
	p := put(t, root, "-a", "id1", now, lnCwd, lnTitle)
	x := NewIndexer(func() []string { return []string{root} })
	reads := 0
	real := x.read
	x.read = func(path string, off int64, n int) ([]byte, error) { reads++; return real(path, off, n) }
	x.Scan()
	first := reads
	x.Scan()
	if first == 0 || reads != first {
		t.Fatalf("reads: first scan %d, after second %d", first, reads)
	}
	os.WriteFile(p, []byte(lnCwd+"\n"+`{"type":"ai-title","aiTitle":"Baru"}`+"\n"), 0o644)
	os.Chtimes(p, now.Add(time.Minute), now.Add(time.Minute))
	if es := x.Scan(); es[0].Title != "Baru" {
		t.Fatalf("changed file not re-read: %+v", es[0])
	}
}

func TestScanBigFileReadsBoundedBytes(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".claude", "projects")
	filler := strings.Repeat(`{"type":"summary","pad":"`+strings.Repeat("x", 500)+`"}`+"\n", 40000) // ~20 MB
	p := put(t, root, "-a", "big", now, lnCwd)
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(filler + lnTitle + "\n")
	f.Close()
	x := NewIndexer(func() []string { return []string{root} })
	total := 0
	real := x.read
	x.read = func(path string, off int64, n int) ([]byte, error) {
		b, err := real(path, off, n)
		total += len(b)
		return b, err
	}
	es := x.Scan()
	if total > 64<<10+256<<10 {
		t.Fatalf("read %d bytes", total)
	}
	if es[0].Title != "Kerangka halaman toko" || es[0].Cwd != `D:\webshop` {
		t.Fatalf("%+v", es[0])
	}
}

func TestTitleFallbacks(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".claude", "projects")
	put(t, root, "-a", "custom", now.Add(3*time.Second), lnCwd, `{"type":"custom-title","customTitle":"Nama saya"}`)
	put(t, root, "-a", "prompt", now.Add(2*time.Second), lnCwd)
	put(t, root, "-a", "abcdef123456", now.Add(time.Second), lnAsst)
	got := map[string]string{}
	for _, e := range scan(root) {
		got[e.ID] = e.Title
	}
	if got["custom"] != "Nama saya" || got["prompt"] != "tolong cek halaman toko" || got["abcdef123456"] != "abcdef12" {
		t.Fatalf("%v", got)
	}
}

func TestScanMissingRootsAndGarbage(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".claude", "projects")
	put(t, root, "-a", "empty", now)
	put(t, root, "-a", "junk", now, "{not json", "\x00\x01\x02")
	es := scan(root, filepath.Join(root, "nope"))
	if len(es) != 2 {
		t.Fatalf("%+v", es)
	}
	if es := scan(); len(es) != 0 {
		t.Fatal("no roots must give no entries")
	}
}

func TestFoldersGroupByCwd(t *testing.T) {
	es := []Entry{
		{ID: "1", Cwd: "/a", ModTime: now, Account: "personal"},
		{ID: "2", Cwd: "/a", ModTime: now.Add(-time.Hour), Account: "work"},
		{ID: "3", Cwd: "/b", ModTime: now.Add(time.Hour)},
		{ID: "4", Cwd: ""},
	}
	fs := Folders(es)
	if len(fs) != 3 || fs[0].Cwd != "/b" || fs[1].Cwd != "/a" || fs[1].Count != 2 || !fs[1].Latest.Equal(now) || fs[2].Cwd != "" {
		t.Fatalf("%+v", fs)
	}
}

func TestFolderName(t *testing.T) {
	for in, want := range map[string]string{`D:\webshop`: "webshop", "/home/k/p/x": "x", "/home/k/p/x/": "x", "": "?", "/": "?"} {
		if got := FolderName(in); got != want {
			t.Errorf("%q: %q want %q", in, got, want)
		}
	}
}

func TestCwdFoundInTailWhenHeadHasNone(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".claude", "projects")
	big := `{"type":"summary","pad":"` + strings.Repeat("x", 70<<10) + `"}`
	put(t, root, "-a", "late", now, big, lnAsst, lnCwd)
	if es := scan(root); len(es) != 1 || es[0].Cwd != `D:\webshop` {
		t.Fatalf("%+v: a session whose first lines are huge must still get its cwd", es)
	}
}
