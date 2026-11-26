package session

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSyncHoldReleasesWholeRedraws(t *testing.T) {
	var mu sync.Mutex
	var got []string
	h := &syncHold{emit: func(p []byte) { mu.Lock(); got = append(got, string(p)); mu.Unlock() }}
	seen := func() string { mu.Lock(); defer mu.Unlock(); return strings.Join(got, "|") }

	h.feed([]byte("a\x1b[?20")) // the marker is split across reads
	h.feed([]byte("26hclear"))
	if seen() != "a" {
		t.Fatalf("half a redraw leaked: %q", seen())
	}
	h.feed([]byte("draw\x1b[?2026lz"))
	if want := "a|\x1b[?2026hcleardraw\x1b[?2026lz"; seen() != want {
		t.Fatalf("got %q", seen())
	}

	got = nil
	h.feed([]byte("\x1b[?2026hstuck")) // never closed: must not freeze the screen
	time.Sleep(3 * syncMaxHold)
	if seen() != "\x1b[?2026hstuck" {
		t.Fatalf("an unclosed redraw was never shown: %q", seen())
	}
}
