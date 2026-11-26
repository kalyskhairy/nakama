package workspace

import (
	"strings"
	"testing"

	"github.com/kalyskhairy/nakama/internal/ui"

	uv "github.com/charmbracelet/ultraviolet"
)

func testView() View {
	return View{
		Rects:  Rects{Active: Rect{2, 1, 30, 6}, Tree: Rect{2, 7, 30, 15}, Strip: Rect{40, 1, 60, 2}, ChatHead: Rect{40, 3, 60, 1}, Chat: Rect{40, 4, 60, 20}},
		Active: []ActiveRow{{ID: "a", Folder: "webshop", Title: "Judul yang sangat panjang sekali sampai melewati lebar panel", State: ui.StateWaiting}, {ID: "b", Folder: "nakama", Title: "Stage 3", State: ui.StateWorking, Claimed: true}},
		Tree:   []TreeRow{{Folder: true, Cwd: "/a", Title: "nakama", Count: 2, Expanded: true}, {ID: "s1", Title: "Stage 3 office", State: ui.StateWorking, Account: "work"}},
		Pane:   PaneTree, TreeSel: 1, NewFolders: 2,
		Strip: [2]string{"Explore · opus · bekerja", "alat: Read"}, ChatPlaceholder: "Enter untuk membuka",
	}
}

func text(buf *uv.ScreenBuffer, y, x0, x1 int) string {
	var sb strings.Builder
	for x := x0; x < x1; x++ {
		if c := buf.CellAt(x, y); c != nil && c.Content != "" {
			sb.WriteString(c.Content)
		} else {
			sb.WriteByte(' ')
		}
	}
	return sb.String()
}

func TestDrawNeverWritesOutsideRects(t *testing.T) {
	buf := uv.NewScreenBuffer(120, 30)
	v := testView()
	Draw(&buf, v)
	rs := []Rect{v.Rects.Active, v.Rects.Tree, v.Rects.Strip, v.Rects.ChatHead, v.Rects.Chat}
	for y := 0; y < 30; y++ {
		for x := 0; x < 120; x++ {
			inside := false
			for _, r := range rs {
				inside = inside || r.Contains(x, y)
			}
			if c := buf.CellAt(x, y); !inside && c != nil && c.Content != "" && c.Content != " " {
				t.Fatalf("cell %d,%d outside every rect holds %q", x, y, c.Content)
			}
		}
	}
}

func TestDrawShowsPanelsTitlesAndPlaceholder(t *testing.T) {
	buf := uv.NewScreenBuffer(120, 30)
	Draw(&buf, testView())
	all := ""
	for y := 0; y < 30; y++ {
		all += text(&buf, y, 0, 120) + "\n"
	}
	for _, want := range []string{"Aktif (2)", "Folder / Sesi (+2 baru)", "Chat", "Explore · opus · bekerja", "alat: Read", "Enter untuk membuka", "nakama (2)", "[work]", "webshop"} {
		if !strings.Contains(all, want) {
			t.Errorf("screen lacks %q", want)
		}
	}
}

func TestDrawMarksSelectedRowReverse(t *testing.T) {
	buf := uv.NewScreenBuffer(120, 30)
	v := testView() // tree row 1 selected, tree pane focused
	Draw(&buf, v)
	row := v.Rects.Tree.Y + 1 + 1 // top border, then row index 1
	c := buf.CellAt(v.Rects.Tree.X+3, row)
	if c == nil || c.Style.Attrs&uv.AttrReverse == 0 {
		t.Fatalf("selected row not reversed: %+v", c)
	}
	if c := buf.CellAt(v.Rects.Tree.X+3, row-1); c != nil && c.Style.Attrs&uv.AttrReverse != 0 {
		t.Fatal("unselected row reversed")
	}
}

func TestDrawTruncatesLongTitles(t *testing.T) {
	buf := uv.NewScreenBuffer(120, 30)
	v := testView()
	Draw(&buf, v)
	line := text(&buf, v.Rects.Active.Y+1, v.Rects.Active.X, v.Rects.Active.X+v.Rects.Active.W)
	if !strings.Contains(line, "…") || strings.Contains(line, "lebar panel") {
		t.Fatalf("%q", line)
	}
	if got := text(&buf, v.Rects.Active.Y+1, v.Rects.Active.X+v.Rects.Active.W, v.Rects.Active.X+v.Rects.Active.W+3); strings.TrimSpace(got) != "" {
		t.Fatalf("text spilled past the panel: %q", got)
	}
}

func TestScrollOffsetKeepsSelectionVisible(t *testing.T) {
	for sel, want := range map[int]int{0: 0, 3: 0, 4: 1, 9: 6} {
		if got := ScrollOffset(sel, 4); got != want {
			t.Errorf("sel %d: %d want %d", sel, got, want)
		}
	}
}

func TestStripLines(t *testing.T) {
	got := StripLines("Explore", "opus", ui.StateWorking, 41, "Read", 2, "")
	if got[0] != "Explore · opus · bekerja · konteks 41%" || got[1] != "alat: Read · 2 subagen" {
		t.Fatalf("%q", got)
	}
	got = StripLines("x", "", ui.StateWaiting, -1, "", 0, "Enter untuk membuka")
	if got[0] != "x · menunggu izin" || got[1] != "Enter untuk membuka" {
		t.Fatalf("%q", got)
	}
}

func TestDrawOverlayClearsAndStaysInsideItsRect(t *testing.T) {
	buf := uv.NewScreenBuffer(60, 20)
	for y := 0; y < 20; y++ {
		for x := 0; x < 60; x++ {
			buf.SetCell(x, y, &uv.Cell{Content: "#", Width: 1})
		}
	}
	r := Rect{10, 4, 30, 8}
	DrawOverlay(&buf, r, "Bantuan", []string{"baris satu", "baris dua yang sangat panjang sekali sampai terpotong", "3", "4", "5", "6", "7 tidak muat"})
	if got := text(&buf, 5, 11, 21); got != " baris sat" && !strings.HasPrefix(strings.TrimSpace(got), "baris") {
		t.Fatalf("line inside overlay: %q", got)
	}
	if c := buf.CellAt(r.X+r.W, r.Y+1); c == nil || c.Content != "#" {
		t.Fatalf("overlay wrote right of its rect: %+v", c)
	}
	if c := buf.CellAt(r.X+2, r.Y+r.H-2); c == nil || c.Content == "#" {
		t.Fatalf("the old picture still shows inside the overlay: %+v", c)
	}
	if strings.Contains(text(&buf, 5, 0, 60), "7 tidak muat") || strings.Contains(text(&buf, 11, 0, 60), "7 tidak") {
		t.Fatal("lines beyond the box height must be cut")
	}
	if !strings.Contains(text(&buf, 4, 0, 60), "Bantuan") {
		t.Fatal("title missing")
	}
}

func TestDrawEmptyTreeSaysHowToFillIt(t *testing.T) {
	buf := uv.NewScreenBuffer(120, 30)
	v := testView()
	v.Tree = nil
	Draw(&buf, v)
	if got := text(&buf, v.Rects.Tree.Y+1, v.Rects.Tree.X, v.Rects.Tree.X+v.Rects.Tree.W); !strings.Contains(got, "f = pilih folder") {
		t.Fatalf("%q", got)
	}
}

func TestDrawTagsAccountsBeforeTheTitle(t *testing.T) {
	buf := uv.NewScreenBuffer(120, 30)
	v := testView()
	v.Active = []ActiveRow{{ID: "a", Folder: "webshop", Account: "work", Title: "Judul yang sangat panjang sekali sampai lewat"}}
	v.Tree = []TreeRow{{ID: "s", Account: "personal", Title: "Judul yang sangat panjang sekali sampai lewat"}}
	Draw(&buf, v)
	if got := text(&buf, v.Rects.Active.Y+1, v.Rects.Active.X, v.Rects.Active.X+v.Rects.Active.W); !strings.Contains(got, "[work] webshop") {
		t.Fatalf("Aktif: %q", got)
	}
	if got := text(&buf, v.Rects.Tree.Y+1, v.Rects.Tree.X, v.Rects.Tree.X+v.Rects.Tree.W); !strings.Contains(got, "[personal] Judul") {
		t.Fatalf("tree: a long title must not push the tag off: %q", got)
	}
	v.Account = "work"
	Draw(&buf, v)
	if got := text(&buf, v.Rects.Tree.Y, v.Rects.Tree.X, v.Rects.Tree.X+v.Rects.Tree.W); !strings.Contains(got, "· work") {
		t.Fatalf("the title must say the tree is filtered: %q", got)
	}
}
