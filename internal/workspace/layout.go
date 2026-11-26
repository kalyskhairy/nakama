// Package workspace is the three-panel screen: where the panels go, which rows they hold and how
// they are drawn.
package workspace

type Rect struct{ X, Y, W, H int }

func (r Rect) Contains(x, y int) bool { return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H }

// Rects places every panel. Chat is the area the terminal of the open session fills, so its size
// is the size of that session's PTY. A Rect of zero size is not shown.
type Rects struct{ Active, Tree, Strip, ChatHead, Chat, Status Rect }

const narrow = 100

func clamp(v, lo, hi int) int { return max(lo, min(v, hi)) }

func Compute(w, h, activeN int, zoom, chatFocus bool) Rects {
	r := Rects{Status: Rect{0, h - 1, w, 1}}
	if zoom || (w < narrow && chatFocus) {
		r.ChatHead, r.Chat = Rect{0, 0, w, 1}, Rect{0, 1, w, h - 2}
		return r
	}
	left := w
	if w >= narrow {
		left = clamp(w/4, 28, 40)
	}
	ah := clamp(activeN+2, 3, max((h-1)/3, 3))
	r.Active, r.Tree = Rect{0, 0, left, ah}, Rect{0, ah, left, h - 1 - ah}
	if w >= narrow {
		rw := w - left
		r.Strip, r.ChatHead, r.Chat = Rect{left, 0, rw, 2}, Rect{left, 2, rw, 1}, Rect{left, 3, rw, h - 4}
	}
	return r
}
