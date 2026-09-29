package ports

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos/macostest"
)

const lsofListening = `p501
cnode
f22
n*:3000
f23
n[::1]:3000
f24
n127.0.0.1:9229
p77
cpostgres
f5
n127.0.0.1:5432
p663
cControlCenter
f10
n*:7000
f12
n*:5000
`

func TestParseListenersMergesSocketsPerProcess(t *testing.T) {
	processes := parseListeners(lsofListening)

	if len(processes) != 3 {
		t.Fatalf("got %d processes, want 3", len(processes))
	}
	node := processes[0]
	if node.PID != 501 || node.Name != "node" {
		t.Errorf("first process = %d %q, want 501 node", node.PID, node.Name)
	}
	if !slices.Equal(node.Ports, []int{3000, 9229}) {
		t.Errorf("node ports = %v, want [3000 9229]", node.Ports)
	}
	if want := []string{"*:3000", "[::1]:3000", "127.0.0.1:9229"}; !slices.Equal(node.Addresses, want) {
		t.Errorf("node addresses = %v, want %v", node.Addresses, want)
	}
	if !slices.Equal(processes[2].Ports, []int{5000, 7000}) {
		t.Errorf("ControlCenter ports = %v, want sorted [5000 7000]", processes[2].Ports)
	}
}

func TestParseElapsed(t *testing.T) {
	tests := []struct {
		value string
		want  time.Duration
	}{
		{"00:07", 7 * time.Second},
		{"12:34", 12*time.Minute + 34*time.Second},
		{"03:00:01", 3*time.Hour + time.Second},
		{"2-01:00:00", 49 * time.Hour},
	}
	for _, tt := range tests {
		got, err := parseElapsed(tt.value)
		if err != nil || got != tt.want {
			t.Errorf("parseElapsed(%q) = %v, %v; want %v", tt.value, got, err, tt.want)
		}
	}
	for _, bad := range []string{"", "7", "a:b", "x-01:00"} {
		if _, err := parseElapsed(bad); err == nil {
			t.Errorf("parseElapsed(%q) succeeded, want an error", bad)
		}
	}
}

func TestParseProcessInfoKeepsSpacesInExecutable(t *testing.T) {
	infos := parseProcessInfo("  501   01:02 /Applications/Visual Studio Code.app/Contents/MacOS/Electron\n")

	info, ok := infos[501]
	if !ok {
		t.Fatal("pid 501 missing")
	}
	if info.executable != "/Applications/Visual Studio Code.app/Contents/MacOS/Electron" || info.elapsed != 62*time.Second {
		t.Errorf("info = %+v", info)
	}
}

func TestListDescribesProcesses(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(home, "projects", "site")
	workDir := filepath.Join(repo, "apps", "web")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &macostest.Runner{Outputs: map[string]string{
		"lsof -nP -iTCP -sTCP:LISTEN -Fpcn":         lsofListening,
		"ps -ww -o pid=,etime=,comm= -p 501,77,663": "501 01:00:00 /opt/homebrew/bin/node\n77 3-00:00:00 /opt/homebrew/opt/postgresql/bin/postgres\n663 10:00 /System/Library/CoreServices/ControlCenter.app/Contents/MacOS/ControlCenter\n",
		"ps -ww -o pid=,command= -p 501,77,663":     "501 node /x/next dev\n77 postgres -D /var\n663 ControlCenter\n",
		"lsof -a -nP -d cwd -Fpn -p 501,77,663":     "p501\nfcwd\nn" + workDir + "\np77\nfcwd\nn/\n",
	}}

	processes, err := Lister{Runner: runner, Home: home}.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	names := []string{processes[0].Name, processes[1].Name, processes[2].Name}
	if !slices.Equal(names, []string{"node", "ControlCenter", "postgres"}) {
		t.Fatalf("order = %v, want sorted by lowest port", names)
	}
	node := processes[0]
	if node.Project != repo {
		t.Errorf("project = %q, want the git root %q", node.Project, repo)
	}
	if node.CommandLine != "node /x/next dev" || node.Executable != "/opt/homebrew/bin/node" {
		t.Errorf("node = %+v", node)
	}
	if uptime := node.Uptime(time.Now()); uptime < time.Hour || uptime > time.Hour+time.Minute {
		t.Errorf("uptime = %v, want about an hour", uptime)
	}
	if processes[2].Project != "" {
		t.Errorf("postgres project = %q, want none for a process running in /", processes[2].Project)
	}
	if !processes[1].IsSystem() || processes[1].Tag() != "airplay" {
		t.Errorf("ControlCenter system=%v tag=%q, want a system process tagged airplay", processes[1].IsSystem(), processes[1].Tag())
	}
}

func TestListTreatsNoMatchesAsEmpty(t *testing.T) {
	exitOne := exec.Command("sh", "-c", "exit 1").Run()
	runner := &macostest.Runner{Errors: map[string]error{"lsof -nP -iTCP -sTCP:LISTEN -Fpcn": exitOne}}

	processes, err := Lister{Runner: runner}.List(context.Background())
	if err != nil || len(processes) != 0 {
		t.Errorf("List = %v, %v; want nothing and no error", processes, err)
	}
}

func TestListReportsLsofFailures(t *testing.T) {
	runner := &macostest.Runner{Errors: map[string]error{"lsof -nP -iTCP -sTCP:LISTEN -Fpcn": exec.ErrNotFound}}

	if _, err := (Lister{Runner: runner}).List(context.Background()); err == nil {
		t.Error("List succeeded, want the lsof error")
	}
}

func TestIsExposed(t *testing.T) {
	tests := []struct {
		addresses []string
		want      bool
	}{
		{[]string{"127.0.0.1:3000", "[::1]:3000"}, false},
		{[]string{"127.0.0.1:3000", "*:3000"}, true},
		{[]string{"192.168.1.5:8080"}, true},
	}
	for _, tt := range tests {
		if got := (Process{Addresses: tt.addresses}).IsExposed(); got != tt.want {
			t.Errorf("IsExposed(%v) = %v, want %v", tt.addresses, got, tt.want)
		}
	}
}

func TestStopTerminatesGracefully(t *testing.T) {
	p := startProcess(t, "sleep 60")

	result := Stopper{Runner: macos.ExecRunner{}}.Stop(context.Background(), p)

	if result.Err != nil || result.Outcome != Terminated {
		t.Errorf("result = %v, %v; want terminated", result.Outcome, result.Err)
	}
}

func TestStopKillsProcessesIgnoringSIGTERM(t *testing.T) {
	p := startProcess(t, "trap '' TERM; exec sleep 60")

	result := Stopper{Runner: macos.ExecRunner{}, Grace: 200 * time.Millisecond}.Stop(context.Background(), p)

	if result.Err != nil || result.Outcome != Killed {
		t.Errorf("result = %v, %v; want killed", result.Outcome, result.Err)
	}
}

func TestStopRefusesAReusedPid(t *testing.T) {
	p := startProcess(t, "sleep 60")
	p.StartedAt = p.StartedAt.Add(-time.Hour)

	result := Stopper{Runner: macos.ExecRunner{}}.Stop(context.Background(), p)

	if !errors.Is(result.Err, ErrProcessReplaced) {
		t.Errorf("err = %v, want ErrProcessReplaced", result.Err)
	}
	if !isAlive(p.PID) {
		t.Error("the process was signalled although it did not match")
	}
}

func TestStopRefusesProtectedProcesses(t *testing.T) {
	for _, pid := range []int{0, 1, os.Getpid()} {
		result := Stopper{Runner: macos.ExecRunner{}}.Stop(context.Background(), Process{PID: pid})
		if !errors.Is(result.Err, ErrProtectedProcess) {
			t.Errorf("pid %d: err = %v, want ErrProtectedProcess", pid, result.Err)
		}
	}
}

func TestStopReportsAlreadyExited(t *testing.T) {
	p := startProcess(t, "sleep 60")
	if err := unixKill(p.PID); err != nil {
		t.Fatal(err)
	}
	waitUntilGone(t, p.PID)

	result := Stopper{Runner: macos.ExecRunner{}}.Stop(context.Background(), p)

	if result.Err != nil || result.Outcome != AlreadyExited {
		t.Errorf("result = %v, %v; want already exited", result.Outcome, result.Err)
	}
}

// startProcess runs script in the background, reaps it when it exits so it does not linger
// as a zombie, and describes it the way the Lister would.
func startProcess(t *testing.T, script string) Process {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	pid := cmd.Process.Pid
	var info processInfo
	for range 50 {
		out, _ := exec.Command("ps", "-ww", "-o", "pid=,etime=,comm=", "-p", strconv.Itoa(pid)).Output()
		info = parseProcessInfo(string(out))[pid]
		if info.executable != "" && info.executable != "sh" && info.executable != "/bin/sh" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	return Process{PID: pid, Name: "sleep", Executable: info.executable, StartedAt: time.Now().Add(-info.elapsed)}
}

func unixKill(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

func waitUntilGone(t *testing.T, pid int) {
	t.Helper()
	for range 100 {
		if !isAlive(pid) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("pid %d did not exit", pid)
}

func TestHolding(t *testing.T) {
	processes := parseListeners(lsofListening)

	holders, free := Holding(processes, []int{3000, 9229, 8080})

	if len(holders) != 1 || holders[0].PID != 501 {
		t.Errorf("holders = %v, want node once", holders)
	}
	if !slices.Equal(free, []int{8080}) {
		t.Errorf("free = %v, want [8080]", free)
	}
}

func TestProcessKinds(t *testing.T) {
	tests := []struct {
		process    Process
		wantTag    string
		wantSystem bool
	}{
		{Process{Name: "node", Executable: "/opt/homebrew/bin/node"}, "", false},
		{Process{Name: "rapportd", Executable: "/usr/libexec/rapportd"}, "system", true},
		{Process{Name: "siriactionsd", Executable: "/Library/Developer/CoreSimulator/Volumes/iOS_22G86/iOS 18.6.simruntime/Contents/siriactionsd"}, "simulator", true},
		{Process{Name: "ControlCenter", Executable: "/System/Library/CoreServices/ControlCenter.app/Contents/MacOS/ControlCenter"}, "airplay", true},
		{Process{Name: "com.docker.backend", Executable: "/Applications/Docker.app/Contents/MacOS/com.docker.backend"}, "docker", false},
	}
	for _, tt := range tests {
		if got := tt.process.Tag(); got != tt.wantTag {
			t.Errorf("%s: tag = %q, want %q", tt.process.Name, got, tt.wantTag)
		}
		if got := tt.process.IsSystem(); got != tt.wantSystem {
			t.Errorf("%s: system = %v, want %v", tt.process.Name, got, tt.wantSystem)
		}
		if (tt.process.Caution() == "") != (tt.wantTag == "") {
			t.Errorf("%s: caution %q does not match tag %q", tt.process.Name, tt.process.Caution(), tt.wantTag)
		}
	}
}
