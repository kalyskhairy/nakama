package live

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	os.MkdirAll(filepath.Join(dir, "sessions"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, "sessions", name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fakeAlive(t *testing.T, alive ...int) {
	old := isAlive
	isAlive = func(pid int) bool {
		for _, p := range alive {
			if p == pid {
				return true
			}
		}
		return false
	}
	t.Cleanup(func() { isAlive = old })
}

func TestReadReturnsOnlyLiveProcesses(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "10.json", `{"pid":10,"sessionId":"s-live","cwd":"/home/k/x","status":"idle","name":"x-f7"}`)
	write(t, dir, "11.json", `{"pid":11,"sessionId":"s-dead","cwd":"/home/k/y"}`)
	fakeAlive(t, 10)
	got := Read([]string{dir})
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	p := got[0]
	if p.PID != 10 || p.SessionID != "s-live" || p.Cwd != "/home/k/x" || p.Status != "idle" || p.Name != "x-f7" || p.ConfigDir != dir {
		t.Fatalf("%+v", p)
	}
}

func TestReadSkipsBrokenJSONAndMissingDir(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "1.json", `{not json`)
	write(t, dir, "2.json", `{"pid":0,"sessionId":""}`)
	fakeAlive(t, 1, 2, 0)
	if got := Read([]string{dir, filepath.Join(dir, "nope")}); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestBySessionAndByPID(t *testing.T) {
	ps := []Proc{{PID: 1, SessionID: "a"}, {PID: 2, SessionID: "b"}}
	if BySession(ps)["b"].PID != 2 {
		t.Fatal("BySession")
	}
	if p, ok := ByPID(ps, 1); !ok || p.SessionID != "a" {
		t.Fatal("ByPID")
	}
	if _, ok := ByPID(ps, 9); ok {
		t.Fatal("unknown pid found")
	}
}

func TestProcAliveRealProcess(t *testing.T) {
	if procAlive(os.Getpid()) { // the test binary is not named claude
		t.Fatal("a non-claude process must not count as a live claude")
	}
	if procAlive(999999999) {
		t.Fatal("missing pid is alive")
	}
}

func TestABackgroundJobBelongsToTheSessionThatParkedIt(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "10.json", `{"pid":10,"sessionId":"s-old","kind":"interactive","status":"idle","name":"mine","parkedJobId":"j1"}`)
	write(t, dir, "20.json", `{"pid":20,"sessionId":"s-job","kind":"bg","jobId":"j1","status":"busy","name":"Some title"}`)
	write(t, dir, "30.json", `{"pid":30,"sessionId":"s-other","kind":"bg","jobId":"j2","status":"busy"}`) // nobody parked it
	fakeAlive(t, 10, 20, 30)
	got := BySession(Read([]string{dir}))
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	if p := got["s-job"]; p.PID != 10 || p.Status != "busy" || p.Name != "mine" {
		t.Fatalf("the job takes the seat of the session that parked it: %+v", p)
	}
	if _, ok := got["s-old"]; ok {
		t.Fatal("the parked id no longer runs")
	}
	if got["s-other"].PID != 30 {
		t.Fatalf("%+v", got)
	}
}
