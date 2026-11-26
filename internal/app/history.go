package app

import (
	"fmt"
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
	for i := usageDays - 1; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		n := m.report.Days[day.Format("2006-01-02")]
		out = append(out, fmt.Sprintf("  %s  %10s  %s", day.Format("Mon 02 Jan"), ui.Tokens(n), strings.Repeat("█", n*30/most)))
	}
	today := m.report.Sessions(now.Format("2006-01-02"))
	byAccount := map[string]int{}
	for _, s := range today {
		byAccount[s.Account] += s.Tokens
	}
	var accounts []string
	for a := range byAccount {
		accounts = append(accounts, a)
	}
	sort.Strings(accounts)
	out = append(out, "", "Today by account")
	for _, a := range accounts {
		out = append(out, fmt.Sprintf("  %-10s %10s", a, ui.Tokens(byAccount[a])))
	}
	out = append(out, "", "Today by session")
	for _, s := range today[:min(len(today), usageTop)] {
		folder, title := m.title(now, s.ID, s.Cwd)
		out = append(out, fmt.Sprintf("  %10s  [%s] %s · %s", ui.Tokens(s.Tokens), s.Account, folder, title))
	}
	if len(today) == 0 {
		out = append(out, "  nothing yet")
	}
	return append(out, "", "Tokens = input + cache writes + output. ↑/↓ scroll, esc closes.")
}

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
