package workspace

import (
	"fmt"
	"strings"

	"github.com/kalyskhairy/nakama/internal/input"

	"github.com/charmbracelet/x/ansi"
)

// HelpLines is the help screen: every key (from input.Bindings, so it never drifts), what the
// marks and the status bar mean, how the screen works, where things are stored, and tips.
func HelpLines(width int) []string {
	var raw []string
	add := func(s ...string) { raw = append(raw, s...) }
	add("nakama — help   (esc, q or F1 closes, ↑/↓ scroll)", "")
	last := ""
	for _, b := range input.Bindings {
		if b.Context != last {
			add("", b.Context)
			last = b.Context
		}
		add(fmt.Sprintf("  %-16s %s", b.Keys, b.Desc))
	}
	add("", "Marks",
		"  ?  needs your approval or answer     ●  working     ◐  active     ○  idle     ✓  done",
		"  In the office, dim blue characters are subagents; full colour ones are chat sessions.",
		"  !  context 80% full or more: consider /compact or a new session",
		"  (other terminal)  the session is in use elsewhere: view only, never resumed",
		"  just active  the transcript changed seconds ago with no known process: Enter asks to confirm",
		"",
		"Status bar",
		"  ! N waiting  agents (subagents too) waiting for you — most important, cut last",
		"  5h / 7h       plan usage over 5 hours and 7 days, with reset times; per account if more than one",
		"  tok           tokens today of the sessions in Active or opened by nakama",
		"",
		"How it works",
		"  Every Claude Code session on this machine is read from ~/.claude*/projects, grouped by folder.",
		"  Choose the folders shown with f; new folders stay hidden until you do.",
		"  Waiting and running sessions stay in Active even when their folder is hidden.",
		"  Enter on an old session runs `claude --resume` in its own folder and account. A session used by",
		"  another terminal is not resumed, so two processes never write one transcript.",
		"  Accounts: ~/.claude is personal, ~/.claude-work is work. A new session uses the account most",
		"  used in that folder; Tab in the picker changes it.",
		"",
		"Files and flags",
		"  ~/.config/nakama/folders.json  folder choice     ~/.config/nakama/themes/  office themes",
		"  nakama --offline  no network calls (plan limits are not shown)",
		"  --prefix N  change the focus key (byte; default 0 = Ctrl+Space)   --theme NAME   --graphics auto|sixel|off",
		"  --font WxH  terminal cell size for the office   --office-max-scale 1   --context-window 200000",
		"",
		"Tips",
		"  Shift+drag copies text while the mouse is on; m turns the mouse off.",
		"  z zooms the chat to full screen; Ctrl+Space returns.",
		"  Inside a chat every key (esc too) belongs to Claude, except Ctrl+Space and F1.")
	var out []string
	for _, l := range raw {
		if l == "" {
			out = append(out, "")
			continue
		}
		out = append(out, strings.Split(ansi.Wrap(l, max(width, 10), " "), "\n")...)
	}
	return out
}

// ScrollHelp is the page of lines starting at offset, clamped so the last page stays full.
func ScrollHelp(lines []string, offset, h int) []string {
	offset = max(min(offset, len(lines)-h), 0)
	return lines[offset:min(offset+h, len(lines))]
}
