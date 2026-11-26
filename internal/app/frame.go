package app

import (
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/kalyskhairy/nakama/internal/index"
	"github.com/kalyskhairy/nakama/internal/input"
	"github.com/kalyskhairy/nakama/internal/live"
	"github.com/kalyskhairy/nakama/internal/monitor"
	"github.com/kalyskhairy/nakama/internal/office"
	"github.com/kalyskhairy/nakama/internal/session"
	"github.com/kalyskhairy/nakama/internal/ui"
	"github.com/kalyskhairy/nakama/internal/workspace"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

const (
	minW = 40
	minH = 10
)

func putText(buf *uv.ScreenBuffer, y int, text string, style uv.Style) {
	x := 0
	for _, r := range text {
		w := ansi.StringWidth(string(r))
		if w == 0 {
			continue
		}
		buf.SetCell(x, y, &uv.Cell{Content: string(r), Width: w, Style: style})
		x += w
	}
}

func padTo(s string, w int) string {
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

const (
	panelMin, panelMax = 30, 48 // columns of a crew panel beside the picture
	belowMin, belowMax = 6, 14  // rows of a crew panel under it
	typingPause        = time.Second
	chatPictureGap     = 500 * time.Millisecond
)

// crewLines lists the active sessions, with the card of the selected agent at the bottom: exactly
// height lines of exactly width columns, so a shorter list leaves nothing stale on the screen.
func (m *Model) crewLines(width, height int, now time.Time) []string {
	rows := m.crewRows(now)
	var list []string
	sel := 0 // the list line of the selected agent
	side := width >= 2*cardW
	footW := width
	if side {
		footW = cardW
	}
	for _, r := range rows {
		mark := " "
		if m.selected == r.ID {
			mark, sel = ">", len(list)
		}
		who := charName(m.cast[r.ID])
		if who != "" {
			who += " · "
		}
		list = append(list, fmt.Sprintf("%s%s %s[%s] %s", mark, ui.Glyph(r.State), who, r.Account, r.Folder), "    "+r.Title)
		in := m.info(r.ID, now)
		if to := m.visiting(r.ID, in, now); to != "" {
			name := charName(m.cast[to])
			if name == "" {
				name = in.SentTo
			}
			list = append(list, "    → message to "+name)
		}
		if in.PendingTool != "" && (r.State == ui.StateWorking || r.State == ui.StateWaiting) {
			list = append(list, "    tool: "+in.PendingTool)
		}
		if !untracked(r.ID) {
			for _, sub := range m.mon.Subagents(r.ID, now) {
				mark := " "
				if m.selected == "sub:"+r.ID+":"+sub.ID {
					mark, sel = ">", len(list)
				}
				st := ui.StateIdle
				switch {
				case sub.Info.Status == monitor.Waiting:
					st = ui.StateWaiting
				case sub.Info.Status == monitor.Working:
					st = ui.StateWorking
				case sub.Info.Status == monitor.Active:
					st = ui.StateActive
				}
				list = append(list, fmt.Sprintf("%s  └%s %s · %s", mark, ui.Glyph(st), sub.Type, sub.Description))
			}
		}
	}
	foot := []string{"", " Tab select · v back"}
	if c, ok := m.cards[m.selected]; ok {
		foot = []string{strings.Repeat("─", footW), " SELECTED AGENT", ""}
		if side {
			foot = foot[1:]
		}
		title := strings.Split(ansi.Wrap(c.title, max(footW-2, 10), " "), "\n")
		for _, l := range title[:min(len(title), 3)] {
			foot = append(foot, " "+l)
		}
		foot = append(foot, "", " "+c.role)
		for _, f := range c.facts {
			foot = append(foot, " "+f)
		}
		foot = append(foot, "", " Enter = talk in chat")
	}
	room := height - 1 // list lines under the header
	if !side {
		room -= len(foot)
	}
	room = max(room, 0)
	if first := min(sel, len(list)-room); first > 0 { // scroll so the selected one stays in sight
		list = list[first:]
	}
	list = append(list, make([]string, max(room-len(list), 0))...)[:room]
	out := append([]string{fmt.Sprintf(" Crew (%d)", len(rows))}, list...)
	if side {
		for i := range out {
			card := ""
			if i < len(foot) {
				card = foot[i]
			}
			out[i] = padTo(out[i], width-cardW-1) + "│" + padTo(card, cardW)
		}
	} else {
		out = append(out, foot...)
	}
	out = append(out, make([]string, max(height-len(out), 0))...)[:height]
	for i := range out {
		out[i] = padTo(out[i], width)
	}
	return out
}

// officeFrame paints the office picture (the whole picture every frame), a crew panel in the space
// the picture leaves (or the card rows under it when there is none) and the status line. It reports
// false when the picture cannot be drawn at this size.
func (m *Model) officeFrame(w, h int, now time.Time) (string, bool) {
	view := m.views[m.view]
	bg := view.th.Background.Bounds()
	fit := func(cols, rows int) (office.Layout, bool) {
		return office.FitMax(cols, rows, m.office.CellW, m.office.CellH, bg.Dx(), bg.Dy(), m.office.MaxScale)
	}
	rows := h - 2 // one spare row: sixel bands are padded to 6 px, and cells may be smaller than assumed
	full, ok := fit(w, rows)
	if !ok {
		return "", false
	}
	side, below, cardRows := 0, 0, 0
	cl, hasChat := m.officeLayout(w, h)
	switch {
	case hasChat:
	case w-full.Cols >= panelMin:
		side = min(w-full.Cols, panelMax)
	case rows-full.Rows >= belowMin:
		below = min(rows-full.Rows, belowMax)
	case m.cardOpen:
		cardRows = 3
	}
	lay, ok := fit(w-side, rows-below-cardRows)
	if hasChat {
		lay, ok = fit(cl.shipCols, cl.shipRows)
	}
	if !ok {
		return "", false
	}
	m.shipLay = lay
	var panel []string
	panelRow, panelCol := 1, w-side+1
	switch {
	case hasChat && cl.crew.W > 0:
		panel, panelRow, panelCol = m.crewLines(cl.crew.W, cl.crew.H, now), cl.crew.Y+1, cl.crew.X+1
	case hasChat:
	case side > 0:
		panel = m.crewLines(side, h-1, now)
	case below > 0: // one blank row under the picture, for the same reason as the spare row
		panel, panelRow, panelCol = m.crewLines(w, below, now), rows-below+2, 1
	}
	placed := m.world.Placed()
	card := [3]string{" no agent selected (Tab to select)", "", ""}
	if c, ok := m.cards[m.selected]; ok {
		card = c.rows()
	}
	status := ui.StatusLine(m.Message(now), m.Summary(now), w)
	var head string
	var chat []string
	var pty *session.Session
	if hasChat {
		head, chat, pty = m.officeChat(cl, now)
	}
	key := fmt.Sprintf("%d,%d,%d,%d,%d,%d,%d,%v,%d,%v", w, h, lay.Col, lay.Row, side, below, cardRows, m.cardOpen, m.view, hasChat)
	imgSig := fmt.Sprintf("%v|%s", placed, m.selected)
	sig := fmt.Sprintf("%v|%v|%s|%s|%v|%v", card, panel, status, head, chat, m.mode)
	redraw := key != m.lastKey || !m.drewOffice
	if !redraw && sig == m.lastSig && imgSig == m.imgSig {
		return "", true // nothing moved, nothing changed: the screen already shows this
	}
	// The picture is by far the largest part (tens of ms to encode, and keystrokes wait for it): send it
	// only when it changed; with the focus in the chat, not while keys arrive and at most twice a second.
	busy := m.Typing() && (now.Sub(m.lastTyped) < typingPause || now.Sub(m.lastImage) < chatPictureGap)
	// Most pictures differ only where somebody moved: then just those parts are sent, which is cheap
	// enough to go on while typing.
	var im *image.NRGBA
	var parts []image.Rectangle
	if !redraw && imgSig != m.imgSig && m.office.CellKnown && m.lastPic != nil {
		im = view.r.Compose(placed, m.selected, lay.Scale)
		tileH := m.office.CellH // sixel rows come in sixes, so a part is a whole number of both
		for tileH%6 != 0 {
			tileH += m.office.CellH
		}
		parts = office.Dirty(m.lastPic, im, 4*m.office.CellW, tileH)
		if len(parts) == 0 { // looks the same: nothing to send
			m.imgSig = imgSig
		}
		area := 0
		for _, r := range parts {
			area += r.Dx() * r.Dy()
		}
		if area*5 > im.Bounds().Dx()*im.Bounds().Dy()*2 { // over 40%: one whole picture is cheaper
			parts = nil
		}
		if area == 0 {
			parts = nil
		}
	}
	picture := redraw || (imgSig != m.imgSig && parts == nil && !busy)
	var sb strings.Builder
	sb.WriteString("\x1b[?25l")
	if redraw {
		sb.WriteString("\x1b[2J")
	}
	if picture || parts != nil {
		pal, err := view.r.Palette()
		if err != nil {
			return "", false
		}
		if im == nil {
			im = view.r.Compose(placed, m.selected, lay.Scale)
		}
		if picture {
			parts = []image.Rectangle{im.Bounds()}
		}
		for _, r := range parts {
			sx, err := office.EncodePart(im, r, pal)
			if err != nil {
				return "", false
			}
			fmt.Fprintf(&sb, "\x1b[%d;%dH", lay.Row+r.Min.Y/m.office.CellH, lay.Col+r.Min.X/m.office.CellW)
			sb.Write(sx)
		}
		m.imgSig, m.lastPic = imgSig, im
		if picture {
			m.lastImage = now
		}
	}
	m.lastKey, m.lastSig, m.drewOffice = key, sig, true
	if redraw && hasChat && cl.crew.X == 0 && cl.head.Y == 0 { // two columns: a rule between them
		for y := 1; y < h; y++ {
			fmt.Fprintf(&sb, "\x1b[%d;%dH\x1b[2m│\x1b[0m", y, cl.chat.X)
		}
	}
	for i, line := range panel {
		fmt.Fprintf(&sb, "\x1b[%d;%dH%s", panelRow+i, panelCol, line)
	}
	if cardRows > 0 {
		for i, line := range card {
			fmt.Fprintf(&sb, "\x1b[%d;1H\x1b[7m%s\x1b[0m", h-cardRows+i, padTo(line, w))
		}
	}
	if hasChat {
		style := "\x1b[2m"
		if m.mode == input.Chat {
			style = "\x1b[7m"
		}
		fmt.Fprintf(&sb, "\x1b[%d;%dH%s%s\x1b[0m", cl.head.Y+1, cl.head.X+1, style, head)
		for i := 0; i < cl.chat.H; i++ { // erase first: a rendered row may be shorter than the panel
			fmt.Fprintf(&sb, "\x1b[%d;%dH\x1b[0m\x1b[%dX", cl.chat.Y+1+i, cl.chat.X+1, cl.chat.W)
			if i < len(chat) {
				sb.WriteString(chat[i])
			}
		}
	}
	fmt.Fprintf(&sb, "\x1b[%d;1H\x1b[0m\x1b[7m%s\x1b[0m", h, status)
	if pty != nil && m.mode == input.Chat {
		if x, y, hidden := pty.Cursor(); !hidden && x < cl.chat.W && y < cl.chat.H {
			fmt.Fprintf(&sb, "\x1b[%d;%dH\x1b[?25h", cl.chat.Y+1+y, cl.chat.X+1+x)
		}
	}
	return sb.String(), true
}

// strip is the two lines about the selected session above the chat.
func (m *Model) strip(now time.Time, id string, s *session.Session) [2]string {
	if id == "" {
		return [2]string{"No session selected", "Pick a session in the list; Enter opens it"}
	}
	e, _ := m.entry(now, id)
	in := m.info(id, now)
	hint := ""
	if _, running := live.BySession(m.procs)[id]; s == nil && running {
		hint = "in use in another terminal: view only"
	} else if s == nil {
		hint = "Enter opens this session (resume)"
	}
	subs := 0
	if !untracked(id) {
		subs = len(m.mon.Subagents(id, now))
	}
	return workspace.StripLines(e.Title, strings.TrimPrefix(in.Model, "claude-"), m.source(now).State(id), in.CtxPct, in.PendingTool, subs, hint)
}

// workView builds what workspace.Draw paints, and where.
func (m *Model) workView(w, h int, now time.Time) (workspace.View, workspace.Rects, *session.Session) {
	active, ai, tree, ti := m.selection(now)
	rects := workspace.Compute(w, h, len(active), m.zoom, m.mode == input.Chat)
	s := m.shown(now)
	id := m.selectedID(now)
	newFolders := 0
	for _, f := range index.Folders(m.entries) {
		if m.prefs.IsNew(f.Cwd) {
			newFolders++
		}
	}
	pane := m.pane
	if m.mode == input.Chat {
		pane = workspace.PaneChat
	}
	v := workspace.View{Rects: rects, Active: active, ActiveSel: ai, Tree: tree, TreeSel: ti, Pane: pane, Query: m.query,
		NewFolders: newFolders, Account: m.account, Strip: m.strip(now, id, s)}
	if s == nil {
		v.ChatPlaceholder = "Enter opens the selected session"
	}
	return v, rects, s
}

// Frame returns the ANSI string that paints the whole screen.
func Frame(m *Model, w, h int, now time.Time) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	if w < minW || h < minH {
		m.drewOffice = false
		msg := ansi.Truncate(fmt.Sprintf("terminal too small (min %dx%d)", minW, minH), w, "")
		return "\x1b[?25l\x1b[2J\x1b[1;1H" + msg
	}
	m.syncSizes(now)
	if m.officeShown() {
		if f, ok := m.officeFrame(w, h, now); ok {
			return f
		}
	}
	buf := uv.NewScreenBuffer(w, h)
	v, rects, s := m.workView(w, h, now)
	workspace.Draw(&buf, v)
	if m.mode == input.Home { // the office was asked for but its picture does not fit
		putText(&buf, 0, " office: too small for the picture, press v to go back", uv.Style{Attrs: uv.AttrReverse})
	}
	var cx, cy int
	showCursor := false
	if s != nil && rects.Chat.W > 0 {
		c := rects.Chat
		s.Draw(buf, uv.Rect(c.X, c.Y, c.W, c.H))
		x, y, hidden := s.Cursor()
		cx, cy, showCursor = c.X+x, c.Y+y, m.mode == input.Chat && !hidden && x < c.W && y < c.H
	}
	if m.mode == input.Overlay {
		title, lines, r := m.overlayContent(w, h, now)
		workspace.DrawOverlay(&buf, r, title, lines)
	}
	putText(&buf, h-1, ui.StatusLine(m.Message(now), m.Summary(now), w), uv.Style{Attrs: uv.AttrReverse})

	var sb strings.Builder
	sb.WriteString("\x1b[?25l")
	if m.drewOffice { // leaving the office: the picture would stay on the screen
		sb.WriteString("\x1b[2J")
		m.drewOffice = false
	}
	for y, line := range strings.Split(buf.Render(), "\n") {
		if y >= h {
			break
		}
		fmt.Fprintf(&sb, "\x1b[%d;1H%s\x1b[0m\x1b[K", y+1, line)
	}
	if showCursor {
		fmt.Fprintf(&sb, "\x1b[%d;%dH\x1b[?25h", cy+1, cx+1)
	} else {
		sb.WriteString("\x1b[?25l")
	}
	return sb.String()
}

// overlayContent is the title, the lines and the place of the open overlay.
func (m *Model) overlayContent(w, h int, now time.Time) (string, []string, workspace.Rect) {
	bw, bh := min(w-4, 110), h-3
	r := workspace.Rect{X: (w - bw) / 2, Y: 1, W: bw, H: bh}
	iw, ih := bw-2, bh-2
	switch m.overlay {
	case ovFilter:
		return "Folder", m.filter.Lines(iw, ih), r
	case ovNew:
		return "New session", m.newPick.Lines(iw, ih), r
	case ovDetail:
		return "Details: " + m.detailTitle(now), m.detailLines(now), r
	}
	lines := workspace.HelpLines(iw - 2)
	m.helpScroll = max(min(m.helpScroll, len(lines)-ih), 0)
	return "Help", workspace.ScrollHelp(lines, m.helpScroll, ih), r
}

func (m *Model) detailTitle(now time.Time) string {
	e, _ := m.entry(now, m.detailID)
	return e.Title
}

func (m *Model) detailLines(now time.Time) []string {
	e, _ := m.entry(now, m.detailID)
	in := m.info(m.detailID, now)
	ctx := "?"
	if in.CtxPct >= 0 {
		ctx = fmt.Sprintf("%d%%", in.CtxPct)
	}
	out := []string{
		fmt.Sprintf("%s · %s · account %s", e.Title, index.FolderName(e.Cwd), e.Account),
		fmt.Sprintf("status %s · context %s · tokens today %d", workspace.StateWord(m.source(now).State(m.detailID)), ctx, in.TokensToday),
		"", "Latest events (newest first), esc closes:",
	}
	acts := m.mon.Activity(m.detailID)
	for _, a := range acts {
		out = append(out, a.Time.Local().Format("15:04:05")+"  "+a.Text)
	}
	if len(acts) == 0 {
		out = append(out, "(no events read yet; this session is not being followed)")
	}
	return out
}
