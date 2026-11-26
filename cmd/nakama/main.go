// nakama runs several interactive `claude` sessions inside one terminal.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/kalyskhairy/nakama/internal/app"
	"github.com/kalyskhairy/nakama/internal/index"
	"github.com/kalyskhairy/nakama/internal/limits"
	"github.com/kalyskhairy/nakama/internal/live"
	"github.com/kalyskhairy/nakama/internal/monitor"
	"github.com/kalyskhairy/nakama/internal/office"
	"github.com/kalyskhairy/nakama/internal/prefs"
	"github.com/kalyskhairy/nakama/internal/session"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

const askCellSize = "\x1b[16t"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "theme" {
		if err := runTheme(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "nakama:", err)
			os.Exit(1)
		}
		return
	}
	cmdName := flag.String("cmd", "claude", "program to run in each session")
	dir := flag.String("dir", ".", "folder the new-session picker offers first")
	prefix := flag.Int("prefix", 0, "byte of the focus key between chat and lists (0 = Ctrl+Space, 0x1d = Ctrl+])")
	offline := flag.Bool("offline", false, "never ask the network for plan limits")
	notify := allNotices()
	flag.Var(notify, "notify", "bell and Windows notification for: all, none, or a list of waiting,finished,context")
	window := flag.Int("context-window", 200000, "context window (tokens) used for the context percentage")
	themeName := flag.String("theme", "default", "office theme (a folder in the themes directory)")
	themesDir := flag.String("themes-dir", defaultThemesDir(), "where the office themes live")
	graphics := flag.String("graphics", "auto", "office picture: auto, sixel (force) or off")
	maxScale := flag.Float64("office-max-scale", 1, "largest scale of the office picture (smaller = cheaper to draw; 1 = full size)")
	font := flag.String("font", "", "terminal cell size in pixels as WxH, when the terminal does not report it")
	flag.Parse()

	in, out := int(os.Stdin.Fd()), int(os.Stdout.Fd())
	if !term.IsTerminal(in) || !term.IsTerminal(out) {
		fmt.Fprintln(os.Stderr, "nakama needs an interactive terminal")
		os.Exit(1)
	}
	W, H, err := term.GetSize(out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	old, err := term.MakeRaw(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cfg := loadOffice(officeOpts{themeName: *themeName, themesDir: *themesDir, graphics: *graphics, font: *font, fd: out, maxScale: *maxScale}, detectSixel(in))
	os.Stdout.WriteString("\x1b[?1049h\x1b[2J\x1b[?2004h") // alt screen, bracketed paste

	if _, _, known := office.CellSize(out, *font); !known {
		os.Stdout.WriteString(askCellSize) // the reply arrives as input; until then the small guess is used
	}

	var restoreOnce sync.Once
	restore := func() {
		restoreOnce.Do(func() {
			term.Restore(in, old)
			os.Stdout.WriteString("\x1b[?1000l\x1b[?1006l\x1b[?2004l\x1b[?25h\x1b[?1049l")
		})
	}
	fatal := func(r any) { // a goroutine panicked: leave the terminal usable, then die
		restore()
		fmt.Fprintf(os.Stderr, "nakama: panic: %v\n", r)
		os.Exit(2)
	}

	var mu sync.Mutex
	dirty := make(chan struct{}, 1)
	markDirty := func() {
		select {
		case dirty <- struct{}{}:
		default:
		}
	}
	m := app.NewModel(func(r app.SpawnReq, w, h int) (*session.Session, error) {
		args, id, env := launchArgs(*cmdName, r)
		d := r.Dir
		if d == "" {
			d = *dir
		}
		return session.Start(session.Config{Cmd: *cmdName, Args: args, Dir: d, W: w, H: h, ID: id, Env: env}, markDirty)
	}, W, H)
	m.SetToggle(byte(*prefix))
	m.SetPrefs(prefs.Load(prefs.DefaultPath()))
	byName, order := map[string]string{}, []string{}
	for _, root := range monitor.Roots() {
		cfg := filepath.Dir(root)
		name := limits.AccountName(cfg)
		byName[name] = cfg
		order = append(order, name)
	}
	m.SetAccounts(byName, order)
	if abs, err := filepath.Abs(*dir); err == nil {
		m.SetDefaultDir(abs)
	}
	m.SetMonitor(monitor.NewFileMonitor(monitor.Roots, *window))
	m.SetOffice(cfg)

	defer func() {
		r := recover()
		runLocked(&mu, func() {
			for _, s := range m.Sessions() {
				s.Close()
			}
		})
		restore()
		if r != nil {
			panic(r)
		}
	}()

	quit := make(chan struct{})
	var quitOnce sync.Once
	stop := func() { quitOnce.Do(func() { close(quit) }) }
	safeGo(fatal, func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				stop()
				return
			}
			var done bool
			runLocked(&mu, func() {
				m.Input(buf[:n], time.Now())
				done = m.Quitting()
			})
			if done {
				stop()
				return
			}
			markDirty()
		}
	})

	m.SetHistoryRoots(monitor.Roots)
	m.SetBackground(func(work func() func()) { // usage and search read every transcript
		safeGo(fatal, func() {
			apply := work()
			runLocked(&mu, apply)
			markDirty()
		})
	})

	// every second: read what the transcripts gained
	safeGo(fatal, func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-quit:
				return
			case <-t.C:
				runLocked(&mu, func() { m.Poll(time.Now()) })
				markDirty()
			}
		}
	})

	// every 5 seconds: the sessions on this machine and the claude processes that run
	idx := index.NewIndexer(monitor.Roots)
	safeGo(fatal, func() {
		for {
			es := idx.Scan()
			var cfgs []string
			for _, root := range monitor.Roots() {
				cfgs = append(cfgs, filepath.Dir(root))
			}
			ps := live.Read(cfgs)
			runLocked(&mu, func() { m.SetIndex(es); m.SetLive(ps) })
			markDirty()
			select {
			case <-quit:
				return
			case <-time.After(5 * time.Second):
			}
		}
	})

	// plan limits: the poller itself allows one request a minute per account
	home, _ := os.UserHomeDir()
	poller := limits.NewPoller(&http.Client{Timeout: 10 * time.Second}, "https://api.anthropic.com", limits.Accounts(home), *offline)
	safeGo(fatal, func() {
		for {
			poller.Refresh(context.Background(), time.Now())
			l := poller.Latest()
			runLocked(&mu, func() { m.SetLimits(l) })
			markDirty()
			select {
			case <-quit:
				return
			case <-time.After(10 * time.Second):
			}
		}
	})

	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	// a closed terminal or a kill must end the sessions too, not only q
	gone := make(chan os.Signal, 1)
	signal.Notify(gone, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)
	tick := time.NewTicker(33 * time.Millisecond)
	defer tick.Stop()
	lastDraw := time.Time{}
	need := true
	for {
		select {
		case <-quit:
			return
		case <-gone:
			return
		case <-winch:
			if w, h, err := term.GetSize(out); err == nil {
				cw, ch, known := office.CellSize(out, *font)
				runLocked(&mu, func() {
					W, H = w, h
					m.Resize(w, h)
					if known {
						m.SetCellSize(cw, ch)
					}
				})
				os.Stdout.WriteString("\x1b[2J")
				if !known {
					os.Stdout.WriteString(askCellSize)
				}
				need = true
			}
		case <-dirty:
			need = true
		case <-tick.C:
			var animating, typing bool
			runLocked(&mu, func() { animating, typing = m.Animating(), m.Typing() })
			gap := time.Duration(0)
			if animating { // the office is a whole picture per frame: at most 16 a second, and a slow frame just delays the next
				gap, need = 60*time.Millisecond, true
				if typing { // keystrokes show at once; the frame itself holds the picture to 8 a second
					gap = 0
				}
			}
			if (need || time.Since(lastDraw) > 500*time.Millisecond) && time.Since(lastDraw) >= gap { // slow refresh expires messages and updates exit marks
				var frame string
				var notices []app.Notice
				runLocked(&mu, func() {
					notices = m.TakeNotices()
					m.Tick(time.Now())
					frame = m.MouseSeq() + app.Frame(m, W, H, time.Now())
				})
				if frame != "" { // shown in one go by terminals that know synchronized output; ignored by others
					os.Stdout.WriteString(ansi.SetModeSynchronizedOutput + frame + ansi.ResetModeSynchronizedOutput)
				}
				rang := false
				for _, n := range notices {
					if !notify.on[n.Kind] {
						continue
					}
					if !rang {
						os.Stdout.WriteString("\a") // here, so it can not land inside a frame
						rang = true
					}
					toast(n)
				}
				lastDraw, need = time.Now(), false
			}
		}
	}
}
