package workspace

import (
	"fmt"
	"strings"

	"github.com/kalyskhairy/nakama/internal/ui"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

type Pane int

const (
	PaneActive Pane = iota
	PaneTree
	PaneChat
)

// View is everything Draw needs. The terminal of the open session is drawn by the caller into
// Rects.Chat, after Draw.
type View struct {
	Rects           Rects
	Active          []ActiveRow
	ActiveSel       int
	Tree            []TreeRow
	TreeSel         int
	Pane            Pane
	Query           string
	NewFolders      int
	Account         string // account the tree is filtered to
	Strip           [2]string
	ChatPlaceholder string // shown in the chat area when no terminal is open there
}

var stateWords = map[ui.State]string{ui.StateWaiting: "needs approval", ui.StateWorking: "working", ui.StateActive: "active", ui.StateIdle: "idle", ui.StateExited: "done"}

func StateWord(st ui.State) string { return stateWords[st] }

// ScrollOffset is the first visible row of a list of `visible` rows that keeps row sel in view.
func ScrollOffset(sel, visible int) int { return max(sel-visible+1, 0) }

// put writes text at x, y inside r, cut to r's width. Nothing is ever written outside r.
func put(buf *uv.ScreenBuffer, r Rect, x, y int, text string, st uv.Style) {
	if y < 0 || y >= r.H || x >= r.W {
		return
	}
	text = ansi.Truncate(text, r.W-x, "…")
	for _, ch := range text {
		w := ansi.StringWidth(string(ch))
		if w == 0 {
			continue
		}
		buf.SetCell(r.X+x, r.Y+y, &uv.Cell{Content: string(ch), Width: w, Style: st})
		x += w
	}
}

func pad(buf *uv.ScreenBuffer, r Rect, y int, st uv.Style) {
	for x := 0; x < r.W; x++ {
		buf.SetCell(r.X+x, r.Y+y, &uv.Cell{Content: " ", Width: 1, Style: st})
	}
}

func box(buf *uv.ScreenBuffer, r Rect, title string, focused bool) {
	if r.W < 2 || r.H < 2 {
		return
	}
	st := uv.Style{}
	if focused {
		st.Attrs = uv.AttrBold
	}
	top := "┌ " + title + " " + strings.Repeat("─", r.W)
	put(buf, r, 0, 0, ansi.Truncate(top, r.W-1, "")+"┐", st)
	for y := 1; y < r.H-1; y++ {
		put(buf, r, 0, y, "│", st)
		put(buf, r, r.W-1, y, "│", st)
	}
	put(buf, r, 0, r.H-1, "└"+strings.Repeat("─", r.W-2)+"┘", st)
}

// tag names the account in front of a title, where a long title cannot push it off.
func tag(account string) string {
	if account == "" {
		return ""
	}
	return "[" + account + "] "
}

func (v View) row(buf *uv.ScreenBuffer, r Rect, i int, text string, selected, focused bool) {
	st := uv.Style{}
	if selected {
		st.Attrs = uv.AttrBold
		if focused {
			st.Attrs = uv.AttrReverse
		}
	}
	inner := Rect{r.X + 1, r.Y + 1, r.W - 2, r.H - 2}
	pad(buf, inner, i, st)
	put(buf, inner, 0, i, text, st)
}

// Draw paints the panels, the strip and the chat header into buf.
func Draw(buf *uv.ScreenBuffer, v View) {
	rs := v.Rects
	if a := rs.Active; a.W > 0 && a.H > 0 {
		box(buf, a, fmt.Sprintf("Active (%d)", len(v.Active)), v.Pane == PaneActive)
		vis := a.H - 2
		off := ScrollOffset(v.ActiveSel, vis)
		for i := 0; i < vis && off+i < len(v.Active); i++ {
			r := v.Active[off+i]
			text := fmt.Sprintf("%s %s%s · %s", ui.Glyph(r.State), tag(r.Account), r.Folder, r.Title)
			if r.Claimed {
				text += " (other terminal)"
			}
			v.row(buf, a, i, text, off+i == v.ActiveSel, v.Pane == PaneActive)
		}
	}
	if t := rs.Tree; t.W > 0 && t.H > 0 {
		title := "Folder / Session"
		if v.Account != "" {
			title += " · " + v.Account
		}
		if v.NewFolders > 0 {
			title += fmt.Sprintf(" (+%d new)", v.NewFolders)
		}
		if v.Query != "" {
			title += " /" + v.Query
		}
		box(buf, t, title, v.Pane == PaneTree)
		if len(v.Tree) == 0 {
			put(buf, Rect{t.X + 1, t.Y + 1, t.W - 2, t.H - 2}, 0, 0, "empty · f = choose folders", uv.Style{Attrs: uv.AttrFaint})
		}
		vis := t.H - 2
		off := ScrollOffset(v.TreeSel, vis)
		for i := 0; i < vis && off+i < len(v.Tree); i++ {
			r := v.Tree[off+i]
			text := ""
			if r.Folder {
				mark := "▸"
				if r.Expanded {
					mark = "▾"
				}
				text = fmt.Sprintf("%s %s (%d)", mark, r.Title, r.Count)
			} else {
				text = fmt.Sprintf("   %s %s%s", ui.Glyph(r.State), tag(r.Account), r.Title)
				if r.Claimed {
					text += " (other terminal)"
				}
			}
			v.row(buf, t, i, text, off+i == v.TreeSel, v.Pane == PaneTree)
		}
	}
	for i, l := range v.Strip {
		put(buf, rs.Strip, 1, i, l, uv.Style{})
	}
	if rs.ChatHead.W > 0 {
		st := uv.Style{Attrs: uv.AttrFaint}
		if v.Pane == PaneChat {
			st.Attrs = uv.AttrBold
		}
		put(buf, rs.ChatHead, 0, 0, "Chat ── Ctrl+Space = list ── z = zoom ── F1 = help", st)
	}
	if c := rs.Chat; v.ChatPlaceholder != "" && c.W > 0 && c.H > 0 {
		x := max((c.W-ansi.StringWidth(v.ChatPlaceholder))/2, 0)
		put(buf, c, x, c.H/2, v.ChatPlaceholder, uv.Style{Attrs: uv.AttrFaint})
	}
}

// StripLines is the two-line summary of the selected session above the chat.
func StripLines(title, model string, st ui.State, ctxPct int, pendingTool string, subagents int, hint string) [2]string {
	l1 := []string{title}
	if model != "" {
		l1 = append(l1, model)
	}
	l1 = append(l1, StateWord(st))
	if ctxPct >= 0 {
		l1 = append(l1, fmt.Sprintf("context %d%%", ctxPct))
	}
	var l2 []string
	if pendingTool != "" {
		l2 = append(l2, "tool: "+pendingTool)
	}
	if subagents > 0 {
		l2 = append(l2, fmt.Sprintf("%d subagents", subagents))
	}
	if len(l2) == 0 && hint != "" {
		l2 = []string{hint}
	}
	return [2]string{strings.Join(l1, " · "), strings.Join(l2, " · ")}
}

// DrawOverlay clears r, draws a titled box in it and the lines inside, over whatever was there.
func DrawOverlay(buf *uv.ScreenBuffer, r Rect, title string, lines []string) {
	if r.W < 4 || r.H < 3 {
		return
	}
	for y := 0; y < r.H; y++ {
		pad(buf, r, y, uv.Style{})
	}
	box(buf, r, title, true)
	inner := Rect{r.X + 1, r.Y + 1, r.W - 2, r.H - 2}
	for i, l := range lines {
		if i >= inner.H {
			break
		}
		put(buf, inner, 1, i, l, uv.Style{})
	}
}
