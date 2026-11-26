package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kalyskhairy/nakama/internal/app"
	"github.com/kalyskhairy/nakama/internal/office"
	"github.com/kalyskhairy/nakama/internal/theme"

	"golang.org/x/sys/unix"
)

type officeOpts struct {
	themeName, themesDir string
	graphics             string // auto | sixel | off
	font                 string // "WxH" cell size when the terminal does not report it
	fd                   int    // terminal file descriptor for the cell-size query
	maxScale             float64
}

func defaultThemesDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "nakama", "themes")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "nakama", "themes")
}

// loadOffice prepares the office home screen. When it cannot, the returned config carries the
// reason and the home screen falls back to the text list.
func loadOffice(o officeOpts, hasSixel func() bool) *app.OfficeConfig {
	fail := func(format string, a ...any) *app.OfficeConfig {
		return &app.OfficeConfig{Reason: fmt.Sprintf(format, a...)}
	}
	if o.graphics == "off" {
		return fail("graphics turned off (--graphics off)")
	}
	th, err := theme.Load(filepath.Join(o.themesDir, o.themeName))
	if err != nil {
		return fail("theme %q: %v", o.themeName, err)
	}
	if o.graphics != "sixel" && !hasSixel() {
		return fail("the terminal does not support sixel (force it with --graphics sixel)")
	}
	r, err := office.NewRenderer(th)
	if err != nil {
		return fail("theme %q: %v", o.themeName, err)
	}
	cw, ch, known := office.CellSize(o.fd, o.font)
	return &app.OfficeConfig{Theme: th, Renderer: r, CellW: cw, CellH: ch, CellKnown: known, MaxScale: o.maxScale}
}

// detectSixel asks the terminal for its attributes (ESC [ c) and waits up to 300 ms for the answer.
// It polls instead of blocking on a read, so no goroutine is left behind to swallow a keypress.
func detectSixel(in int) func() bool {
	return func() bool {
		os.Stdout.WriteString("\x1b[c")
		var resp []byte
		buf := make([]byte, 256)
		for deadline := time.Now().Add(300 * time.Millisecond); time.Now().Before(deadline) && !bytes.Contains(resp, []byte("c")); {
			fds := []unix.PollFd{{Fd: int32(in), Events: unix.POLLIN}}
			if n, _ := unix.Poll(fds, 50); n > 0 {
				if k, _ := unix.Read(in, buf); k > 0 {
					resp = append(resp, buf[:k]...)
				}
			}
		}
		return office.ParseDA1(resp)
	}
}
