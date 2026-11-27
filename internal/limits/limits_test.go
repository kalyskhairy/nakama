package limits

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)

const body = `{"five_hour":{"utilization":52,"resets_at":"2026-11-25T03:40:00Z"},"seven_day":{"utilization":12.5,"resets_at":"2026-11-30T05:00:00Z"}}`

const secret = "SECRET-TOKEN-123"

func account(t *testing.T, home, name string, expires time.Time) string {
	t.Helper()
	dir := filepath.Join(home, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	creds := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":%q,"expiresAt":%d}}`, secret, expires.UnixMilli())
	if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(creds), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

var future = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)

func server(t *testing.T, status int, delay time.Duration, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		if r.URL.Path != "/api/oauth/usage" || r.Header.Get("Authorization") != "Bearer "+secret ||
			r.Header.Get("anthropic-beta") != "oauth-2025-04-20" { // claude-monitor sends the same header
			t.Errorf("request missing the required headers: %v", r.Header)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		time.Sleep(delay)
		w.WriteHeader(status)
		if status == 200 {
			w.Write([]byte(body))
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func TestParseUsage(t *testing.T) {
	five, seven, err := Parse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if five.Pct != 52 || !five.ResetsAt.Equal(time.Date(2026, 11, 25, 3, 40, 0, 0, time.UTC)) || seven.Pct != 12.5 {
		t.Fatalf("%+v %+v", five, seven)
	}
	if _, _, err := Parse([]byte(`{"x":1}`)); err == nil {
		t.Fatal("expected an error for an unknown shape")
	}
}

func TestFetchSendsBearerAndParses(t *testing.T) {
	home := t.TempDir()
	srv := server(t, 200, 0, nil)
	for dir, name := range map[string]string{account(t, home, ".claude", future): "personal", account(t, home, ".claude-work", future): "work"} {
		u := Fetch(context.Background(), srv.Client(), srv.URL, dir, t0)
		if u.Err != "" || u.Account != name || u.FiveHour.Pct != 52 || u.SevenDay.Pct != 12.5 {
			t.Fatalf("%s: %+v", name, u)
		}
	}
}

func TestFetchExpiredTokenIsNotSent(t *testing.T) {
	var hits atomic.Int32
	srv := server(t, 200, 0, &hits)
	dir := account(t, t.TempDir(), ".claude", t0.Add(-time.Hour))
	u := Fetch(context.Background(), srv.Client(), srv.URL, dir, t0)
	if hits.Load() != 0 || !strings.Contains(u.Err, "token expired") {
		t.Fatalf("hits=%d %+v", hits.Load(), u)
	}
}

func TestFetchHTTPErrors(t *testing.T) {
	dir := account(t, t.TempDir(), ".claude", future)
	for status, want := range map[int]string{401: "401", 429: "rate limited", 500: "500"} {
		srv := server(t, status, 0, nil)
		if u := Fetch(context.Background(), srv.Client(), srv.URL, dir, t0); !strings.Contains(u.Err, want) {
			t.Errorf("status %d: %q", status, u.Err)
		}
	}
	slow := server(t, 200, 500*time.Millisecond, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if u := Fetch(ctx, slow.Client(), slow.URL, dir, t0); !strings.Contains(u.Err, "timeout") {
		t.Errorf("slow: %q", u.Err)
	}
	dead := httptest.NewServer(http.NotFoundHandler())
	url := dead.URL
	dead.Close()
	if u := Fetch(context.Background(), http.DefaultClient, url, dir, t0); !strings.Contains(u.Err, "offline") {
		t.Errorf("refused: %q", u.Err)
	}
}

func TestFetchMissingCredentials(t *testing.T) {
	srv := server(t, 200, 0, nil)
	dir := filepath.Join(t.TempDir(), ".claude")
	os.MkdirAll(dir, 0o755)
	if u := Fetch(context.Background(), srv.Client(), srv.URL, dir, t0); !strings.Contains(u.Err, "no credentials") {
		t.Fatalf("%+v", u)
	}
}

func TestPollerRateLimits(t *testing.T) {
	var hits atomic.Int32
	srv := server(t, 200, 0, &hits)
	dir := account(t, t.TempDir(), ".claude", future)
	p := NewPoller(srv.Client(), srv.URL, []string{dir}, false)
	p.Refresh(context.Background(), t0)
	p.Refresh(context.Background(), t0.Add(30*time.Second))
	if hits.Load() != 1 {
		t.Fatalf("hits after 30s = %d", hits.Load())
	}
	p.Refresh(context.Background(), t0.Add(61*time.Second))
	if hits.Load() != 2 {
		t.Fatalf("hits after 61s = %d", hits.Load())
	}
	if l := p.Latest(); len(l) != 1 || l[0].FiveHour.Pct != 52 {
		t.Fatalf("latest %+v", l)
	}
}

func TestPollerRefreshNowHasTenSecondFloor(t *testing.T) {
	var hits atomic.Int32
	srv := server(t, 200, 0, &hits)
	dir := account(t, t.TempDir(), ".claude", future)
	p := NewPoller(srv.Client(), srv.URL, []string{dir}, false)
	p.Refresh(context.Background(), t0)
	p.RefreshNow(context.Background(), t0.Add(5*time.Second))
	if hits.Load() != 1 {
		t.Fatalf("hits = %d", hits.Load())
	}
	p.RefreshNow(context.Background(), t0.Add(11*time.Second))
	if hits.Load() != 2 {
		t.Fatalf("hits = %d", hits.Load())
	}
}

func TestPollerOfflineMakesNoRequests(t *testing.T) {
	var hits atomic.Int32
	srv := server(t, 200, 0, &hits)
	home := t.TempDir()
	dirs := []string{account(t, home, ".claude", future), account(t, home, ".claude-work", future)}
	p := NewPoller(srv.Client(), srv.URL, dirs, true)
	p.Refresh(context.Background(), t0)
	l := p.Latest()
	if hits.Load() != 0 || len(l) != 2 || l[0].Err != "offline" || l[1].Account != "work" {
		t.Fatalf("hits=%d latest=%+v", hits.Load(), l)
	}
}

func TestAccounts(t *testing.T) {
	home := t.TempDir()
	d := account(t, home, ".claude", future)
	w := account(t, home, ".claude-work", future)
	os.MkdirAll(filepath.Join(home, ".claude-x"), 0o755) // no credentials
	got := Accounts(home)
	if len(got) != 2 || got[0] != d || got[1] != w {
		t.Fatalf("got %v", got)
	}
}

func TestTokenNeverAppearsInErrors(t *testing.T) {
	var all []Usage
	dir := account(t, t.TempDir(), ".claude", future)
	for _, st := range []int{401, 429, 500} {
		srv := server(t, st, 0, nil)
		all = append(all, Fetch(context.Background(), srv.Client(), srv.URL, dir, t0))
	}
	srv := server(t, 200, time.Second, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	all = append(all, Fetch(ctx, srv.Client(), srv.URL, dir, t0))
	all = append(all, Fetch(context.Background(), srv.Client(), srv.URL, account(t, t.TempDir(), ".claude", t0.Add(-time.Hour)), t0))
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	all = append(all, Fetch(context.Background(), http.DefaultClient, deadURL, dir, t0))
	for _, u := range all {
		if u.Err == "" || strings.Contains(u.Err, secret) {
			t.Errorf("bad error string: %q", u.Err)
		}
	}
}

func TestARateLimitKeepsTheLastFiguresAndWaitsBeforeAskingAgain(t *testing.T) {
	var hits atomic.Int32
	limited := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if limited {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"five_hour":{"utilization":40},"seven_day":{"utilization":10}}`))
	}))
	t.Cleanup(srv.Close)
	p := NewPoller(srv.Client(), srv.URL, []string{account(t, t.TempDir(), ".claude", future)}, false)
	p.Refresh(context.Background(), t0)
	limited = true
	p.Refresh(context.Background(), t0.Add(61*time.Second))
	if l := p.Latest(); l[0].Err != "" || l[0].FiveHour.Pct != 40 {
		t.Fatalf("the last good figures must stay: %+v", l[0])
	}
	p.Refresh(context.Background(), t0.Add(122*time.Second)) // a minute later, but the server asked for two
	if hits.Load() != 2 {
		t.Fatalf("asked again too soon: %d requests", hits.Load())
	}
	p.Refresh(context.Background(), t0.Add(182*time.Second))
	if hits.Load() != 3 {
		t.Fatalf("never asked again: %d requests", hits.Load())
	}
}

func TestARateLimitBeforeAnyFigureSaysSo(t *testing.T) {
	srv := server(t, http.StatusTooManyRequests, 0, nil)
	p := NewPoller(srv.Client(), srv.URL, []string{account(t, t.TempDir(), ".claude", future)}, false)
	p.Refresh(context.Background(), t0)
	if l := p.Latest(); l[0].Err != "rate limited, retrying later" {
		t.Fatalf("%+v", l[0])
	}
}

func TestWindowsShareOneCheck(t *testing.T) {
	var hits atomic.Int32
	srv := server(t, 200, 0, &hits)
	dirs := []string{account(t, t.TempDir(), ".claude", future)}
	shared := filepath.Join(t.TempDir(), "limits.json")
	a := NewPoller(srv.Client(), srv.URL, dirs, false)
	b := NewPoller(srv.Client(), srv.URL, dirs, false)
	a.Share(shared)
	b.Share(shared)
	a.Refresh(context.Background(), t0)
	b.Refresh(context.Background(), t0.Add(20*time.Second))
	if hits.Load() != 1 {
		t.Fatalf("two windows asked %d times within a minute", hits.Load())
	}
	if l := b.Latest(); len(l) != 1 || l[0].FiveHour.Pct != 52 || !l[0].At.Equal(t0) {
		t.Fatalf("the second window shows the first one's figures: %+v", l)
	}
	b.Refresh(context.Background(), t0.Add(81*time.Second))
	if hits.Load() != 2 {
		t.Fatalf("a minute later one of them asks again: %d", hits.Load())
	}
	if st, err := os.Stat(shared); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("%v %v", st, err)
	}
}

func TestAWaitTheServerAskedForHoldsForEveryWindow(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "300")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)
	dirs := []string{account(t, t.TempDir(), ".claude", future)}
	shared := filepath.Join(t.TempDir(), "limits.json")
	a := NewPoller(srv.Client(), srv.URL, dirs, false)
	b := NewPoller(srv.Client(), srv.URL, dirs, false)
	a.Share(shared)
	b.Share(shared)
	a.Refresh(context.Background(), t0)
	b.Refresh(context.Background(), t0.Add(2*time.Minute))
	if hits.Load() != 1 || b.Latest()[0].Err != "rate limited, retrying later" {
		t.Fatalf("hits %d, %+v", hits.Load(), b.Latest())
	}
}

func TestABrokenSharedFileIsNoReasonToStop(t *testing.T) {
	srv := server(t, 200, 0, nil)
	shared := filepath.Join(t.TempDir(), "limits.json")
	os.WriteFile(shared, []byte("{broken"), 0o600)
	p := NewPoller(srv.Client(), srv.URL, []string{account(t, t.TempDir(), ".claude", future)}, false)
	p.Share(shared)
	p.Refresh(context.Background(), t0)
	if l := p.Latest(); l[0].FiveHour.Pct != 52 {
		t.Fatalf("%+v", l)
	}
}
