package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/kalyskhairy/nakama/internal/input"
	"github.com/kalyskhairy/nakama/internal/live"
	"github.com/kalyskhairy/nakama/internal/session"
	"github.com/kalyskhairy/nakama/internal/workspace"

	uv "github.com/charmbracelet/ultraviolet"
)

const (
	crewW    = 30
	cardW    = 32 // the selected agent's card beside a wide crew list
	chatCols = 80 // Claude needs about this much to look right
)

// officeRects places the office screen with its chat panel. The picture takes shipCols x shipRows
// cells at the top left; the other panels are in cells, 0-based.
type officeRects struct {
	shipCols, shipRows int
	crew, head, chat   workspace.Rect
}

// officeLayout puts, on a wide screen, the ship above the crew on the left and the chat on the
// right; on a tall one, the ship beside the crew above the chat. It reports false when the screen has
// no room for a chat panel.
func (m *Model) officeLayout(w, h int) (officeRects, bool) {
	switch {
	case w >= chatCols+70 && h >= 24:
		cw := max(chatCols, w/2)
		left := w - cw
		crewH := max(14, (h-1)*2/5)
		ship := h - 1 - crewH - 1 // one blank row under the picture
		return officeRects{shipCols: left - 1, shipRows: ship,
			crew: workspace.Rect{X: 0, Y: ship + 1, W: left - 1, H: crewH},
			head: workspace.Rect{X: left, Y: 0, W: cw, H: 1},
			chat: workspace.Rect{X: left, Y: 1, W: cw, H: h - 2}}, true
	case w >= 60 && h >= 30:
		ch := h * 45 / 100
		top := h - ch - 2 // rows above the chat header
		crew := 0
		if w >= chatCols {
			crew = crewW
		}
		return officeRects{shipCols: w - crew, shipRows: top - 1, // one blank row under the picture
			crew: workspace.Rect{X: w - crew, Y: 0, W: crew, H: top},
			head: workspace.Rect{X: 0, Y: top, W: w, H: 1},
			chat: workspace.Rect{X: 0, Y: top + 1, W: w, H: ch}}, true
	}
	return officeRects{}, false
}

// officeChatID is the session the office chat panel is about.
func (m *Model) officeChatID(now time.Time) string {
	if m.mode == input.Chat {
		return m.selectedID(now)
	}
	return m.owner[m.selected]
}

// officeChat is the header and the rows of the chat panel, and the PTY in it, if any.
func (m *Model) officeChat(r officeRects, now time.Time) (head string, rows []string, s *session.Session) {
	id := m.officeChatID(now)
	title, note := "", "Tab memilih kru, lalu Enter untuk bicara"
	if id != "" {
		e, _ := m.entry(now, id)
		title = " " + e.Title + " ──"
		s = m.ownedAll()[id]
		note = "Enter membuka sesi ini (resume)"
		if _, running := live.BySession(m.procs)[id]; running {
			note = "sesi ini dipakai di terminal lain: hanya bisa dilihat"
		}
	}
	hint := "Enter = bicara · z = layar penuh · v = daftar"
	if m.mode == input.Chat {
		hint = "Ctrl+Space = pilih kru"
	}
	head = padTo(fmt.Sprintf("Chat ──%s %s ", title, hint), r.head.W)
	buf := uv.NewScreenBuffer(r.chat.W, r.chat.H)
	if s != nil {
		s.Draw(buf, uv.Rect(0, 0, r.chat.W, r.chat.H))
	} else {
		putText(&buf, r.chat.H/2, "  "+note, uv.Style{Attrs: uv.AttrFaint})
	}
	return head, strings.Split(buf.Render(), "\n"), s
}
