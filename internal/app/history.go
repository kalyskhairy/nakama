package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kalyskhairy/nakama/internal/history"
	"github.com/kalyskhairy/nakama/internal/index"
	"github.com/kalyskhairy/nakama/internal/input"
	"github.com/kalyskhairy/nakama/internal/ui"
)

const (
	usageDays   = 7
	usageTop    = 15
	searchLimit = 50
)

// SetHistoryRoots says where the transcripts of every account are.
func (m *Model) SetHistoryRoots(f func() []string) { m.roots = f }

// SetBackground runs slow work away from the screen: work returns what to apply to the model,
// and the runner applies it under the model's lock. Without one the work runs at once.
func (m *Model) SetBackground(run func(work func() func())) { m.background = run }

func (m *Model) later(work func() func()) {
	if m.background == nil {
		work()()
		return
	}
	m.background(work)
}

func (m *Model) transcriptRoots() []string {
	if m.roots == nil {
		return nil
	}
	return m.roots()
}

func (m *Model) openUsage(now time.Time) {
	m.openOverlay(ovUsage)
	m.helpScroll, m.report = 0, nil
	roots := m.transcriptRoots()
	m.later(func() func() {
		r := history.Usage(roots, now.AddDate(0, 0, 1-usageDays), now.Location())
		return func() { m.report = &r }
	})
}

func (m *Model) title(now time.Time, id, cwd string) (name, title string) {
	if e, ok := m.entry(now, id); ok {
		return index.FolderName(e.Cwd), e.Title
	}
	return index.FolderName(cwd), id
}

func (m *Model) usageLines(now time.Time) []string {
	out := []string{"Plan limits"}
	for _, l := range m.limits {
		if l.Err != "" {
			out = append(out, fmt.Sprintf("  %-10s %s", l.Account, l.Err))
			continue
		}
		out = append(out, fmt.Sprintf("  %-10s 5h %3.0f%% (%s)   7d %3.0f%% (%s)", l.Account,
			l.FiveHour.Pct, resetText(l.FiveHour.ResetsAt, now), l.SevenDay.Pct, resetText(l.SevenDay.ResetsAt, now)))
	}
	if len(m.limits) == 0 {
		out = append(out, "  not available")
	}
	if m.report == nil {
		return append(out, "", "Reading transcripts…")
	}
	out = append(out, "", fmt.Sprintf("Last %d days", usageDays))
	most := 1
	for _, n := range m.report.Days {
		most = max(most, n)
	}
	week := map[string]history.Use{}
	for i := usageDays - 1; i >= 0; i-- {
		day := now.AddDate(0, 0, -i).Format("2006-01-02")
		n := m.report.Days[day]
		out = append(out, fmt.Sprintf("  %s  %10s  %9s  %s", now.AddDate(0, 0, -i).Format("Mon 02 Jan"), ui.Tokens(n), dollars(m.report.Costs[day]), strings.Repeat("█", n*30/most)))
		for a, u := range m.report.Accounts[day] {
			w := week[a]
			week[a] = history.Use{Tokens: w.Tokens + u.Tokens, Cost: w.Cost + u.Cost}
		}
	}
	byAccount := func(title string, uses map[string]history.Use) {
		var names []string
		for a := range uses {
			names = append(names, a)
		}
		sort.Strings(names)
		out = append(out, "", title)
		for _, a := range names {
			out = append(out, fmt.Sprintf("  %-10s %10s  %9s", a, ui.Tokens(uses[a].Tokens), dollars(uses[a].Cost)))
		}
		if len(names) == 0 {
			out = append(out, "  nothing yet")
		}
	}
	byAccount("Today by account", m.report.Accounts[now.Format("2006-01-02")])
	byAccount(fmt.Sprintf("Last %d days by account", usageDays), week)
	today := m.report.Sessions(now.Format("2006-01-02"))
	out = append(out, "", "Today by session")
	for _, s := range today[:min(len(today), usageTop)] {
		folder, title := m.title(now, s.ID, s.Cwd)
		out = append(out, fmt.Sprintf("  %10s  %9s  [%s] %s · %s", ui.Tokens(s.Tokens), dollars(s.Cost), s.Account, folder, title))
	}
	if len(today) == 0 {
		out = append(out, "  nothing yet")
	}
	out = append(out, "", "Tokens = input + cache writes + output.",
		"Cost = what these tokens would cost on the API at list prices (cache reads included).",
		"A Claude subscription is billed differently, so this is a comparison, not your bill.")
	if m.report.Unpriced > 0 {
		out = append(out, fmt.Sprintf("%s of models without a known price are not in any cost.", ui.Tokens(m.report.Unpriced)))
	}
	return append(out, "↑/↓ scroll, esc closes.")
}

// dollars writes an amount like $12.34.
func dollars(d float64) string { return fmt.Sprintf("$%.2f", d) }

func (m *Model) openSearch() {
	m.openOverlay(ovSearch)
	m.searchQuery, m.searched, m.hits, m.hit, m.searching = "", "", nil, 0, false
}

func (m *Model) searchKey(k string, now time.Time) {
	switch k {
	case "esc":
		m.closeOverlay(now)
	case "up":
		m.hit = max(m.hit-1, 0)
	case "down":
		m.hit = max(min(m.hit+1, len(m.hits)-1), 0)
	case "backspace":
		if r := []rune(m.searchQuery); len(r) > 0 {
			m.searchQuery = string(r[:len(r)-1])
		}
	case "enter":
		if m.searchQuery == m.searched && m.hit < len(m.hits) {
			m.openHit(m.hits[m.hit], now)
			return
		}
		m.runSearch()
	default:
		if len([]rune(k)) == 1 && k >= " " {
			m.searchQuery += k
		}
	}
}

func (m *Model) runSearch() {
	q, roots := m.searchQuery, m.transcriptRoots()
	if strings.TrimSpace(q) == "" {
		return
	}
	m.searched, m.hits, m.hit, m.searching = q, nil, 0, true
	m.searchSeq++
	seq := m.searchSeq
	m.later(func() func() {
		hits := history.Search(roots, q, searchLimit)
		return func() {
			if seq == m.searchSeq { // else a newer search is on its way
				m.hits, m.searching = hits, false
			}
		}
	})
}

func (m *Model) openHit(h history.Hit, now time.Time) {
	e, ok := m.entry(now, h.ID)
	if !ok {
		m.say(now, "that session is not in the list yet")
		return
	}
	m.closeOverlay(now)
	m.mode, m.inOffice = input.Work, false
	m.openEntry(e, now)
}

func (m *Model) searchLines(width, height int, now time.Time) []string {
	out := []string{"Type, then Enter. ↑/↓ choose, Enter opens, esc closes.", "> " + m.searchQuery + "▏", ""}
	switch {
	case m.searching:
		return append(out, "Searching…")
	case m.searched != "" && len(m.hits) == 0:
		return append(out, "Nothing said that in any session.")
	}
	per := max((height-len(out))/2, 1) // two lines a hit
	first := max(m.hit-per+1, 0)
	for i := first; i < len(m.hits) && i < first+per; i++ {
		h := m.hits[i]
		mark := " "
		if i == m.hit {
			mark = ">"
		}
		folder, title := m.title(now, h.ID, h.Cwd)
		out = append(out, fmt.Sprintf("%s %s [%s] %s · %s (%d×)", mark, h.At.Local().Format("02 Jan 15:04"), h.Account, folder, title, h.Count),
			"      "+h.Snippet)
	}
	return out
}

// SetHandoffDir says where handoff files go.
func (m *Model) SetHandoffDir(dir string) { m.handoffDir = dir }

// handoff starts a new session in the folder and account of the selected one, told to read what was
// said there: a fresh context that still knows the story.
func (m *Model) handoff(now time.Time) {
	e, ok := m.entry(now, m.selectedID(now))
	switch {
	case !ok:
		m.say(now, "pick a session first")
		return
	case e.Path == "":
		m.say(now, "this session has no transcript yet: nothing to hand over")
		return
	case e.Cwd == "" || !m.statDir(e.Cwd):
		m.say(now, "the folder of this session is gone: %s", e.Cwd)
		return
	}
	m.say(now, "preparing the handoff…")
	dir := m.handoffDir
	m.later(func() func() {
		file, err := writeHandoff(dir, e, now)
		return func() {
			if err != nil {
				m.say(now, "could not prepare the handoff: %v", err)
				return
			}
			m.mode, m.inOffice = input.Work, false
			m.newSession(SpawnReq{Dir: e.Cwd, ConfigDir: e.ConfigDir, Prompt: "This session continues an earlier one. Read " + file +
				": it is what the user and Claude said there (tool output left out). Summarise in a few lines what was done, " +
				"what was decided and what is still open, then wait for my next instruction."}, now)
		}
	})
}

// writeHandoff writes the conversation of a session to <dir>/<id>.md, readable by the owner only.
func writeHandoff(dir string, e index.Entry, now time.Time) (string, error) {
	text, err := history.Conversation(e.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	file := filepath.Join(dir, e.ID+".md")
	head := fmt.Sprintf("# Handoff from an earlier Claude Code session\n\nSession: %s (%s)\nFolder: %s\nAccount: %s\nWritten by nakama on %s.\n\n",
		e.Title, e.ID, e.Cwd, e.Account, now.Format("2006-01-02 15:04"))
	return file, os.WriteFile(file, []byte(head+text), 0o600)
}
