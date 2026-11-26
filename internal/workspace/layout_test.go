package workspace

import "testing"

func overlaps(a, b Rect) bool {
	if a.W == 0 || a.H == 0 || b.W == 0 || b.H == 0 {
		return false
	}
	return a.X < b.X+b.W && b.X < a.X+a.W && a.Y < b.Y+b.H && b.Y < a.Y+a.H
}

func TestComputeWide(t *testing.T) {
	r := Compute(140, 40, 3, false, false)
	if r.Active.W != 35 || r.Chat.W != 105 || r.Chat.H != 36 || r.Status.Y != 39 || r.Active.H != 5 {
		t.Fatalf("%+v", r)
	}
	all := []Rect{r.Active, r.Tree, r.Strip, r.ChatHead, r.Chat, r.Status}
	for i := range all {
		for j := i + 1; j < len(all); j++ {
			if overlaps(all[i], all[j]) {
				t.Fatalf("rects %d and %d overlap: %+v", i, j, r)
			}
		}
	}
	if r.Active.H+r.Tree.H != 39 {
		t.Fatalf("left column must fill everything above the status row: %+v", r)
	}
}

func TestComputeZoom(t *testing.T) {
	r := Compute(140, 40, 3, true, false)
	if r.Chat != (Rect{0, 1, 140, 38}) || r.ChatHead != (Rect{0, 0, 140, 1}) || r.Active.W != 0 || r.Tree.W != 0 || r.Strip.H != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestComputeNarrowFollowsFocus(t *testing.T) {
	list := Compute(80, 30, 2, false, false)
	if list.Active.W != 80 || list.Tree.W != 80 || list.Chat.W != 0 {
		t.Fatalf("%+v", list)
	}
	chat := Compute(80, 30, 2, false, true)
	if chat.Chat != (Rect{0, 1, 80, 28}) || chat.Active.W != 0 {
		t.Fatalf("%+v", chat)
	}
}

func TestComputeActiveHeightClamps(t *testing.T) {
	if h := Compute(140, 40, 0, false, false).Active.H; h != 3 {
		t.Fatalf("0 active: %d", h)
	}
	if h := Compute(140, 40, 50, false, false).Active.H; h != 13 {
		t.Fatalf("50 active: %d", h)
	}
	if w := Compute(300, 40, 1, false, false).Active.W; w != 40 {
		t.Fatalf("left column cap: %d", w)
	}
	if w := Compute(100, 40, 1, false, false).Active.W; w != 28 {
		t.Fatalf("left column floor: %d", w)
	}
}

func TestContains(t *testing.T) {
	r := Rect{2, 3, 4, 5}
	if !r.Contains(2, 3) || !r.Contains(5, 7) || r.Contains(6, 3) || r.Contains(2, 8) || r.Contains(1, 3) {
		t.Fatal("Contains")
	}
}
