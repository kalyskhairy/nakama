// Package ui builds the plain-text pieces of the screen (no styling).
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type State int

const (
	StateIdle State = iota
	StateActive
	StateWorking
	StateWaiting
	StateExited
)

type LimitView struct {
	Account               string
	FivePct, SevenPct     int // -1 = unknown
	FiveReset, SevenReset string
	Err                   string
	Stale                 bool // the figures are from an earlier check
}

type Summary struct {
	Waiting     int
	Limits      []LimitView
	TokensToday int
}

const waitHint = "? = needs approval, or a slow tool already approved"

const hint = "F1 help · Shift+drag copy"

func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > width {
		s = ansi.Truncate(s, width, "…")
	}
	return s + strings.Repeat(" ", width-ansi.StringWidth(s))
}

// Glyph is the one-character mark of a state.
func Glyph(s State) string {
	switch s {
	case StateExited:
		return "✓"
	case StateWaiting:
		return "?"
	case StateWorking:
		return "●"
	case StateActive:
		return "◐"
	}
	return "○"
}

// Tokens writes a token count short.
func Tokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM tok", float64(n)/1e6)
	case n >= 1000:
		return fmt.Sprintf("%.1fK tok", float64(n)/1e3)
	}
	return fmt.Sprintf("%d tok", n)
}

// old marks a figure from an earlier check.
func old(l LimitView) string {
	if l.Stale {
		return "~"
	}
	return ""
}

// limitFive is the first figure of an account (or its error); limitSeven the second, dropped first.
func limitFive(l LimitView, named bool) string {
	name := ""
	if named {
		name = l.Account + " "
	}
	if l.Err != "" {
		return name + l.Err
	}
	five := fmt.Sprintf("%s5h %s%d%%", name, old(l), l.FivePct)
	if l.FiveReset != "" {
		five += " (" + l.FiveReset + ")"
	}
	return five
}

func limitSeven(l LimitView) string {
	if l.Err != "" {
		return ""
	}
	return fmt.Sprintf("7d %s%d%%", old(l), l.SevenPct)
}

// StatusLine is the always-visible bottom row. The waiting count comes first and is the last
// thing to be cut. The right side is filled in priority order: message, the 5h figure of every
// account, their 7d figures, tokens, hint; a piece that does not fit is dropped whole.
func StatusLine(msg string, sum Summary, width int) string {
	left := ""
	if sum.Waiting > 0 {
		left = fmt.Sprintf("! %d waiting ", sum.Waiting)
	}

	room := width - ansi.StringWidth(left)
	take := func(s string) bool { // reserves s on the right side if it still fits
		if s == "" || ansi.StringWidth(s)+2 > room {
			return false
		}
		room -= ansi.StringWidth(s) + 2
		return true
	}

	tail := ""
	if msg != "" && room > 4 { // a message is never dropped: if it does not fit, it is cut
		if ansi.StringWidth(msg)+2 > room {
			msg = ansi.Truncate(msg, room-2, "…")
		}
		take(msg)
		tail = msg
	}
	named := len(sum.Limits) > 1
	fives := make([]string, len(sum.Limits))
	sevens := make([]string, len(sum.Limits))
	for i, l := range sum.Limits {
		if f := limitFive(l, named); take(f) {
			fives[i] = f
		}
	}
	for i, l := range sum.Limits {
		if fives[i] == "" {
			continue
		}
		if sv := limitSeven(l); take(sv) {
			sevens[i] = sv
		}
	}
	tokens := ""
	if sum.TokensToday > 0 {
		if tk := "active " + Tokens(sum.TokensToday); take(tk) {
			tokens = tk
		}
	}
	hintText := hint
	if sum.Waiting > 0 {
		hintText = waitHint
	}
	if msg != "" || !take(hintText) {
		hintText = ""
	}

	var right []string
	var limitParts []string
	for i := range sum.Limits {
		if fives[i] == "" {
			continue
		}
		part := fives[i]
		if sevens[i] != "" {
			part += " " + sevens[i]
		}
		limitParts = append(limitParts, part)
	}
	if len(limitParts) > 0 {
		right = append(right, strings.Join(limitParts, " │ "))
	}
	for _, s := range []string{tokens, tail, hintText} {
		if s != "" {
			right = append(right, s)
		}
	}
	r := strings.Join(right, "  ")
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(r)
	if gap < 1 {
		return fit(left+r, width)
	}
	return fit(left+strings.Repeat(" ", gap)+r, width)
}
