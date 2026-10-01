package claudesessions

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func writeLiveFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadLiveKeepsSessionsOpenInARunningProcess(t *testing.T) {
	dir := t.TempDir()
	writeLiveFile(t, dir, "100.json", `{"pid":100,"sessionId":"working","procStart":"Thu Oct  1 09:12:07 2026","entrypoint":"claude-desktop","status":"busy","statusUpdatedAt":1790845928299}`)
	writeLiveFile(t, dir, "101.json", `{"pid":101,"sessionId":"waiting","procStart":"Thu Oct  1 09:12:08 2026","status":"waiting","waitingFor":"permission"}`)
	writeLiveFile(t, dir, "102.json", `{"pid":102,"sessionId":"exited","procStart":"Thu Oct  1 09:12:09 2026","status":"busy"}`)
	writeLiveFile(t, dir, "103.json", `{"pid":103,"sessionId":"reused-pid","procStart":"Thu Oct  1 09:12:10 2026","status":"busy"}`)
	writeLiveFile(t, dir, "104.json", `{"pid":104,"sessionId":"spare","procStart":"Thu Oct  1 09:12:11 2026","status":"idle","spare":true}`)
	writeLiveFile(t, dir, "105.json", `{"pid":105,"sessionId":"unknown-status","procStart":"Thu Oct  1 09:12:12 2026","status":"compacting"}`)
	writeLiveFile(t, dir, "106.json", `{"pid":106,"sessionId":"half-written`)
	writeLiveFile(t, dir, "106.4e3c.key", `not a status file`)
	var asked []int
	startTimes := func(_ context.Context, pids []int) map[int]string {
		asked = slices.Sorted(slices.Values(pids))
		return map[int]string{
			100: "Thu Oct 1 09:12:07 2026",
			101: "Thu Oct 1 09:12:08 2026",
			103: "Fri Oct 2 08:00:00 2026",
			105: "Thu Oct 1 09:12:12 2026",
		}
	}

	live := ReadLive(context.Background(), dir, startTimes)

	want := map[string]Live{
		"working":        {Activity: Working, Since: time.UnixMilli(1790845928299), PID: 100, Entrypoint: "claude-desktop"},
		"waiting":        {Activity: Waiting, WaitingFor: "permission", PID: 101},
		"unknown-status": {Activity: Idle, PID: 105},
	}
	if len(live) != len(want) {
		t.Errorf("live = %v, want %v", live, want)
	}
	for id, w := range want {
		if got := live[id]; got != w {
			t.Errorf("live[%q] = %+v, want %+v", id, got, w)
		}
	}
	if !slices.Equal(asked, []int{100, 101, 102, 103, 105}) {
		t.Errorf("asked ps about %v", asked)
	}
}

func TestReadLivePrefersTheBusiestProcess(t *testing.T) {
	dir := t.TempDir()
	writeLiveFile(t, dir, "1.json", `{"pid":1,"sessionId":"s","status":"idle","statusUpdatedAt":3000}`)
	writeLiveFile(t, dir, "2.json", `{"pid":2,"sessionId":"s","status":"busy","statusUpdatedAt":1000}`)
	writeLiveFile(t, dir, "3.json", `{"pid":3,"sessionId":"s","status":"busy","statusUpdatedAt":2000}`)
	running := func(context.Context, []int) map[int]string { return map[int]string{1: "a", 2: "b", 3: "c"} }

	if got := ReadLive(context.Background(), dir, running)["s"]; got.PID != 3 {
		t.Errorf("chose %+v, want the latest working process, pid 3", got)
	}
}

func TestReadLiveWithoutStatusFiles(t *testing.T) {
	called := false
	running := func(context.Context, []int) map[int]string { called = true; return nil }

	if live := ReadLive(context.Background(), filepath.Join(t.TempDir(), "missing"), running); len(live) != 0 || called {
		t.Errorf("live = %v, ps called %v", live, called)
	}
}

func TestParseStartTimes(t *testing.T) {
	out := "13507 Thu Oct  1 09:12:07 2026    \n 4044 Tue Sep 29 19:18:46 2026\n\nPID STARTED\n"
	got := parseStartTimes(out)

	want := map[int]string{13507: "Thu Oct 1 09:12:07 2026", 4044: "Tue Sep 29 19:18:46 2026"}
	if len(got) != len(want) || got[13507] != want[13507] || got[4044] != want[4044] {
		t.Errorf("parseStartTimes = %v, want %v", got, want)
	}
}

func TestPSStartTimesFindsThisProcess(t *testing.T) {
	pid := os.Getpid()
	if started := PSStartTimes(context.Background(), []int{pid, pid})[pid]; started == "" {
		t.Error("ps did not report this test's own process")
	}
}

func TestLiveDirFollowsTheConfigDir(t *testing.T) {
	getenv := func(name string) string {
		if name == configDirVariable {
			return "/custom"
		}
		return ""
	}
	if got := LiveDir("/home", getenv); got != "/custom/sessions" {
		t.Errorf("LiveDir = %q", got)
	}
	if got := LiveDir("/home", func(string) string { return "" }); got != "/home/.claude/sessions" {
		t.Errorf("LiveDir = %q", got)
	}
}
