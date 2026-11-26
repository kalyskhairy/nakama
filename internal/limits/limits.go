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
	Err                string // empty on success
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

// Poller refreshes all accounts, at most once a minute (or every 10 s for a manual refresh).
type Poller struct {
	client  *http.Client
	baseURL string
	dirs    []string
	offline bool

	fmu    sync.Mutex // one refresh at a time
	last   time.Time
	mu     sync.Mutex // guards latest
	latest []Usage
}

func NewPoller(client *http.Client, baseURL string, dirs []string, offline bool) *Poller {
	return &Poller{client: client, baseURL: baseURL, dirs: dirs, offline: offline}
}

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
		for _, d := range p.dirs {
			out = append(out, Fetch(ctx, p.client, p.baseURL, d, now))
		}
	}
	p.mu.Lock()
	p.latest = out
	p.mu.Unlock()
}
