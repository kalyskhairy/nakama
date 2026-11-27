// Package limits reads each Claude account's plan limits (5 hour and 7 day windows).
// The token is only read to build one request: it is never refreshed, stored, shown or logged.
package limits

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Window struct {
	Pct      float64
	ResetsAt time.Time
}

type Usage struct {
	Account            string
	FiveHour, SevenDay Window
	Err                string        // empty on success
	At                 time.Time     // when the figures were fetched
	retry              time.Duration // how long the server asked us to wait, after a refusal for asking too often
}

const (
	pollEvery   = 60 * time.Second
	manualFloor = 10 * time.Second
	reqTimeout  = 8 * time.Second
)

// AccountName names a Claude config dir: ~/.claude is "personal", ~/.claude-x is "x".
func AccountName(dir string) string {
	b := filepath.Base(dir)
	if b == ".claude" {
		return "personal"
	}
	return strings.TrimPrefix(b, ".claude-")
}

// Accounts lists ~/.claude and ~/.claude-* that hold a .credentials.json, personal first.
func Accounts(home string) []string {
	cands, _ := filepath.Glob(filepath.Join(home, ".claude-*"))
	cands = append([]string{filepath.Join(home, ".claude")}, cands...)
	var out []string
	for _, c := range cands {
		if _, err := os.Stat(filepath.Join(c, ".credentials.json")); err == nil {
			out = append(out, c)
		}
	}
	return out
}

func parseWindow(raw json.RawMessage) (Window, error) {
	var w struct {
		Utilization *float64 `json:"utilization"`
		ResetsAt    string   `json:"resets_at"`
	}
	if json.Unmarshal(raw, &w) != nil || w.Utilization == nil {
		return Window{}, errors.New("window")
	}
	at, _ := time.Parse(time.RFC3339, w.ResetsAt)
	return Window{Pct: *w.Utilization, ResetsAt: at}, nil
}

func Parse(body []byte) (five, seven Window, err error) {
	var r struct {
		Five  json.RawMessage `json:"five_hour"`
		Seven json.RawMessage `json:"seven_day"`
	}
	if json.Unmarshal(body, &r) != nil {
		return five, seven, errors.New("unrecognised answer")
	}
	if five, err = parseWindow(r.Five); err != nil {
		return five, seven, errors.New("unrecognised answer")
	}
	if seven, err = parseWindow(r.Seven); err != nil {
		return five, seven, errors.New("unrecognised answer")
	}
	return five, seven, nil
}

func readToken(dir string, now time.Time) (string, string) {
	b, err := os.ReadFile(filepath.Join(dir, ".credentials.json"))
	if err != nil {
		return "", "no credentials"
	}
	var c struct {
		O struct {
			Token   string `json:"accessToken"`
			Expires int64  `json:"expiresAt"`
		} `json:"claudeAiOauth"`
	}
	if json.Unmarshal(b, &c) != nil || c.O.Token == "" {
		return "", "no credentials"
	}
	exp := c.O.Expires
	if exp > 0 && exp < 1e12 { // seconds, not milliseconds
		exp *= 1000
	}
	if exp > 0 && exp <= now.UnixMilli() {
		return "", "token expired (open claude)"
	}
	return c.O.Token, ""
}

// Fetch asks the usage endpoint for one account. Err never contains the token.
func Fetch(ctx context.Context, client *http.Client, baseURL, dir string, now time.Time) Usage {
	u := Usage{Account: AccountName(dir)}
	token, msg := readToken(dir, now)
	if msg != "" {
		u.Err = msg
		return u
	}
	ctx, cancel := context.WithTimeout(ctx, reqTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/oauth/usage", nil)
	if err != nil {
		u.Err = "invalid address"
		return u
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20") // required by the OAuth usage endpoint
	req.Header.Set("User-Agent", "nakama")
	resp, err := client.Do(req)
	if err != nil {
		var ne net.Error
		switch {
		case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
			u.Err = "timeout"
		default:
			u.Err = "offline"
		}
		return u
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		u.Err, u.retry = "rate limited, retrying later", retryAfter(resp.Header.Get("Retry-After"))
		return u
	case resp.StatusCode == http.StatusUnauthorized:
		u.Err = "token rejected (HTTP 401)"
		return u
	case resp.StatusCode != http.StatusOK:
		u.Err = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return u
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		u.Err = "timeout"
		return u
	}
	five, seven, err := Parse(data)
	if err != nil {
		u.Err = err.Error()
		return u
	}
	u.FiveHour, u.SevenDay = five, seven
	return u
}

// retryAfter is the wait a Retry-After header asks for in seconds, kept between one and thirty minutes;
// five minutes when it says nothing usable.
func retryAfter(h string) time.Duration {
	s, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || s <= 0 {
		return 5 * time.Minute
	}
	return min(max(time.Duration(s)*time.Second, time.Minute), 30*time.Minute)
}

// Poller refreshes all accounts, at most once a minute (or every 10 s for a manual refresh). Pollers
// that share a file (one per nakama window) ask the server once between them.
type Poller struct {
	client  *http.Client
	baseURL string
	dirs    []string
	offline bool
	shared  string // file the windows share; "" keeps the state in memory only

	fmu    sync.Mutex // one refresh at a time
	last   time.Time
	mu     sync.Mutex // guards latest
	latest []Usage
	mem    map[string]*check
}

// check is what is known about one account dir, the same for every window.
type check struct {
	Checked time.Time `json:"checked"`        // when a window last asked the server
	Held    time.Time `json:"held,omitzero"`  // nobody asks before then: the server said so
	Err     string    `json:"err,omitempty"`  // of the last request
	Good    *Usage    `json:"good,omitempty"` // the last figures that came back
}

func NewPoller(client *http.Client, baseURL string, dirs []string, offline bool) *Poller {
	return &Poller{client: client, baseURL: baseURL, dirs: dirs, offline: offline, mem: map[string]*check{}}
}

// Share keeps the state in a file that other nakama windows read and write too.
func (p *Poller) Share(path string) { p.shared = path }

func (p *Poller) Latest() []Usage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Usage(nil), p.latest...)
}

func (p *Poller) Refresh(ctx context.Context, now time.Time)    { p.refresh(ctx, now, pollEvery) }
func (p *Poller) RefreshNow(ctx context.Context, now time.Time) { p.refresh(ctx, now, manualFloor) }

func (p *Poller) refresh(ctx context.Context, now time.Time, gap time.Duration) {
	p.fmu.Lock()
	defer p.fmu.Unlock()
	var out []Usage
	if p.offline {
		for _, d := range p.dirs {
			out = append(out, Usage{Account: AccountName(d), Err: "offline"})
		}
	} else {
		if !p.last.IsZero() && now.Sub(p.last) < gap {
			return
		}
		p.last = now
		state := p.load()
		for _, d := range p.dirs {
			c := state[d]
			if c == nil {
				c = &check{}
				state[d] = c
			}
			// another window asked a moment ago, or the server asked everyone to wait
			if now.Before(c.Held) || (!c.Checked.IsZero() && now.Sub(c.Checked) < gap) {
				out = append(out, c.show(d))
				continue
			}
			u := Fetch(ctx, p.client, p.baseURL, d, now)
			c.Checked, c.Err = now, u.Err
			if u.retry > 0 {
				c.Held = now.Add(u.retry)
			}
			if u.Err == "" {
				u.At = now
				c.Good = &u
			}
			out = append(out, c.show(d))
		}
		p.save(state)
	}
	p.mu.Lock()
	p.latest = out
	p.mu.Unlock()
}

// show is the figures to put on the screen: the last good ones beat an error.
func (c *check) show(dir string) Usage {
	if c.Good != nil {
		return *c.Good
	}
	return Usage{Account: AccountName(dir), Err: c.Err}
}

// load is the state of every account: this window's, with what other windows found later.
func (p *Poller) load() map[string]*check {
	state := map[string]*check{}
	for d, c := range p.mem {
		state[d] = c
	}
	if p.shared == "" {
		return state
	}
	var file map[string]*check
	if b, err := os.ReadFile(p.shared); err == nil && json.Unmarshal(b, &file) == nil {
		for d, c := range file {
			if mine := state[d]; c != nil && (mine == nil || c.Checked.After(mine.Checked)) {
				state[d] = c
			}
		}
	}
	return state
}

// save keeps the state for the next refresh and, through a rename, for the other windows. A file
// that cannot be written only means the windows ask on their own.
func (p *Poller) save(state map[string]*check) {
	p.mem = state
	if p.shared == "" {
		return
	}
	b, err := json.Marshal(state)
	if err != nil || os.MkdirAll(filepath.Dir(p.shared), 0o700) != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(p.shared), ".limits-*.tmp")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	if cerr := tmp.Close(); werr != nil || cerr != nil || os.Rename(tmp.Name(), p.shared) != nil {
		os.Remove(tmp.Name())
	}
}
