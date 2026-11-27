package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

var noSum = Summary{}

func TestStatusLineWidthExact(t *testing.T) {
	sum := Summary{Waiting: 2, TokensToday: 1234567, Limits: []LimitView{{Account: "personal", FivePct: 78, SevenPct: 39, FiveReset: "02:00"}}}
	for _, w := range []int{12, 20, 40, 80, 200} {
		for _, s := range []Summary{noSum, sum} {
			if got := StatusLine("", s, w); ansi.StringWidth(got) != w {
				t.Errorf("width %d: got %d: %q", w, ansi.StringWidth(got), got)
			}
		}
	}
	if !strings.Contains(StatusLine("", Summary{Waiting: 12345678}, 12), "…") {
		t.Error("width 12 should be truncated with an ellipsis")
	}
}

func TestStatusLineShowsMessageInsteadOfHint(t *testing.T) {
	withMsg := StatusLine("agen 4 no ", noSum, 140)
	if !strings.Contains(withMsg, "agen 4 no ") || strings.Contains(withMsg, "F1 help") {
		t.Errorf("message line: %q", withMsg)
	}
	if !strings.Contains(StatusLine("", noSum, 140), "F1 help") {
		t.Error("hint missing when no message")
	}
}

func TestStatusLineShowsWaitingCount(t *testing.T) {
	if got := StatusLine("", Summary{Waiting: 2}, 140); !strings.Contains(got, "2 waiting") {
		t.Errorf("%q", got)
	}
	if got := StatusLine("", Summary{}, 140); strings.Contains(got, "waiting") {
		t.Errorf("%q", got)
	}
}

func TestStatusLineShowsLimits(t *testing.T) {
	ok := Summary{Limits: []LimitView{{Account: "personal", FivePct: 78, SevenPct: 39, FiveReset: "02:00"}}}
	got := StatusLine("", ok, 160)
	for _, want := range []string{"5h 78%", "7d 39%"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	two := Summary{Limits: []LimitView{
		{Account: "personal", FivePct: 10, SevenPct: 20},
		{Account: "work", Err: "offline", FivePct: -1, SevenPct: -1},
	}}
	got = StatusLine("", two, 200)
	if !strings.Contains(got, "work offline") || !strings.Contains(got, "personal 5h 10%") {
		t.Errorf("two accounts: %q", got)
	}
	if strings.Contains(got, "work 5j") {
		t.Errorf("errored account must show no numbers: %q", got)
	}
}

func TestStatusLineShowsTokens(t *testing.T) {
	for n, want := range map[int]string{1234567: "1.2M", 850: "850 tok", 12300: "12.3K"} {
		if got := StatusLine("", Summary{TokensToday: n}, 160); !strings.Contains(got, want) {
			t.Errorf("%d: want %q in %q", n, want, got)
		}
	}
	if got := StatusLine("", Summary{}, 160); strings.Contains(got, "tok") {
		t.Errorf("zero tokens must not be shown: %q", got)
	}
}

func TestStatusLineKeepsWaitingVisibleWhenNarrow(t *testing.T) {
	sum := Summary{Waiting: 2, TokensToday: 1234567, Limits: []LimitView{{Account: "personal", FivePct: 78, SevenPct: 39}}}
	got := StatusLine("", sum, 40)
	if !strings.Contains(got, "2 waiting") || ansi.StringWidth(got) != 40 {
		t.Errorf("%q (width %d)", got, ansi.StringWidth(got))
	}
}

func TestWaitingHintExplainsSlowTools(t *testing.T) {
	if got := StatusLine("", Summary{Waiting: 1}, 200); !strings.Contains(got, "slow tool") {
		t.Errorf("no explanation next to the waiting count: %q", got)
	}
}

func TestLimitsSurviveTwoAccountsWhenSpaceIsTight(t *testing.T) {
	both := Summary{TokensToday: 5000, Limits: []LimitView{
		{Account: "personal", FivePct: 10, SevenPct: 20},
		{Account: "work", FivePct: 30, SevenPct: 40},
	}}
	// tabs (19) + both 5h figures fit, the 7d figures do not: tokens must not win over limits
	got := StatusLine("", both, 36)
	if !strings.Contains(got, "personal 5h 10%") || !strings.Contains(got, "work 5h 30%") {
		t.Errorf("5h limits of both accounts must come before tokens: %q", got)
	}
	mixed := Summary{TokensToday: 5000, Limits: []LimitView{
		{Account: "personal", FivePct: 10, SevenPct: 20},
		{Account: "work", FivePct: -1, SevenPct: -1, Err: "token expired (open claude)"},
	}}
	got = StatusLine("", mixed, 56)
	if !strings.Contains(got, "personal 5h 10%") || !strings.Contains(got, "work token expired") {
		t.Errorf("limits dropped while tokens stayed: %q", got)
	}
}

func TestSevenDayFigureIsDroppedBeforeFiveDay(t *testing.T) {
	sum := Summary{Limits: []LimitView{{Account: "personal", FivePct: 10, SevenPct: 20}}}
	tight := StatusLine("", sum, 12) // room for the 5h figure only
	if !strings.Contains(tight, "5h 10%") || strings.Contains(tight, "7d") {
		t.Errorf("%q", tight)
	}
}

func TestLongMessageIsTruncatedNotDropped(t *testing.T) {
	msg := "could not start the agent: fork/exec /some/very/long/path/to/claude: permission denied and then some more words"
	got := StatusLine(msg, Summary{}, 60)
	if !strings.Contains(got, "could not start the agent") || !strings.Contains(got, "…") || ansi.StringWidth(got) != 60 {
		t.Fatalf("width %d: %q", ansi.StringWidth(got), got)
	}
	// with tabs taking most of the width the message still gets what is left
	got = StatusLine(msg, Summary{}, 45)
	if !strings.Contains(got, "could not") || ansi.StringWidth(got) != 45 {
		t.Fatalf("width %d: %q", ansi.StringWidth(got), got)
	}
}

func TestOldFiguresAreMarked(t *testing.T) {
	sum := Summary{Limits: []LimitView{{Account: "personal", FivePct: 40, SevenPct: 12, Stale: true}}}
	got := StatusLine("", sum, 80)
	if !strings.Contains(got, "5h ~40%") || !strings.Contains(got, "7d ~12%") {
		t.Fatalf("%q", got)
	}
}
