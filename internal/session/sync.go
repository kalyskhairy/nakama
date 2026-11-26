package session

import (
	"bytes"
	"sync"
	"time"
)

// syncHold keeps back a redraw the child marked as one (synchronized output, mode 2026) until it is
// complete, so a frame is never taken of a half-drawn screen.
type syncHold struct {
	emit func([]byte)

	mu    sync.Mutex
	held  []byte
	timer *time.Timer
}

var syncBegin, syncEnd = []byte("\x1b[?2026h"), []byte("\x1b[?2026l")

const (
	syncMaxHold  = 150 * time.Millisecond // a child that never ends its redraw is shown anyway
	syncMaxBytes = 1 << 20
)

func (h *syncHold) feed(p []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.held = append(h.held, p...)
	cut := len(h.held)
	if i := bytes.LastIndex(h.held, syncBegin); i >= 0 && !bytes.Contains(h.held[i:], syncEnd) {
		cut = i
	} else {
		for n := len(syncBegin) - 1; n > 0; n-- { // the start of a marker split across reads
			if bytes.HasSuffix(h.held, syncBegin[:n]) {
				cut -= n
				break
			}
		}
	}
	if len(h.held)-cut > syncMaxBytes {
		cut = len(h.held)
	}
	h.release(cut)
}

// release emits the first n held bytes and (re)arms the timer for what stays. h.mu is held.
func (h *syncHold) release(n int) {
	if n > 0 {
		h.emit(h.held[:n])
		h.held = append([]byte(nil), h.held[n:]...)
		if h.timer != nil {
			h.timer.Stop()
			h.timer = nil
		}
	}
	if len(h.held) > 0 && h.timer == nil {
		h.timer = time.AfterFunc(syncMaxHold, h.flush)
	}
}

func (h *syncHold) flush() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.release(len(h.held))
}
