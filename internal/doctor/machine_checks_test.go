package doctor

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nasserghiasi/ng-dev-tools/internal/macos"
)

func TestPathFlagsDuplicatesAndMissingDirs(t *testing.T) {
	m := newMachine(t)
	bin := m.dir("bin")
	m.vars["PATH"] = strings.Join([]string{bin, bin, "/definitely/missing", applePathPrefix + "usr/bin"}, ":")

	result := m.run(checkPath)

	assertStatus(t, result, StatusWarn)
	if len(result.Details) != 2 {
		t.Errorf("details = %q, want one duplicate and one missing", result.Details)
	}
}

func TestLocale(t *testing.T) {
	m := newMachine(t)
	assertStatus(t, m.run(checkLocale), StatusWarn)

	m.vars["LANG"] = "en_US.UTF-8"
	assertStatus(t, m.run(checkLocale), StatusPass)

	m.vars["LC_ALL"] = "C"
	assertStatus(t, m.run(checkLocale), StatusWarn)
}

func TestCompetingNodeInstalls(t *testing.T) {
	m := newMachine(t)
	m.dir(".nvm/versions/node")
	assertStatus(t, m.run(checkNodeInstalls), StatusPass)

	if err := os.MkdirAll(filepath.Join(m.root, "opt/homebrew/Cellar/node"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := m.run(checkNodeInstalls)
	assertStatus(t, result, StatusWarn)
	assertContains(t, result.Summary, "nvm, Homebrew")
}

func TestOpenFilesLimit(t *testing.T) {
	m := newMachine(t)
	m.probe.openFiles = 256
	assertStatus(t, m.run(checkOpenFiles), StatusWarn)
}

func TestGitDefaults(t *testing.T) {
	m := newMachine(t)
	m.install("git", "git config --global init.defaultBranch", "main")

	result := m.run(checkGitDefaults)

	assertStatus(t, result, StatusWarn)
	if result.Fix != "git config --global pull.rebase true" {
		t.Errorf("fix = %q", result.Fix)
	}
}

func TestJudgeGitHubSSH(t *testing.T) {
	tests := []struct {
		message string
		want    Status
	}{
		{"Hi octocat! You've successfully authenticated, but GitHub does not provide shell access.", StatusPass},
		{"Host key verification failed.", StatusWarn},
		{"git@github.com: Permission denied (publickey).", StatusFail},
		{"ssh: connect to host github.com port 22: Operation timed out", StatusWarn},
	}
	for _, tt := range tests {
		assertStatus(t, judgeGitHubSSH(tt.message), tt.want)
	}
	assertContains(t, judgeGitHubSSH(tests[0].message).Summary, "octocat")
}

func TestGitHubSSHNeedsAKey(t *testing.T) {
	assertStatus(t, newMachine(t).run(checkGitHubSSH), StatusFail)
}

func TestMisplacedGlobals(t *testing.T) {
	m := newMachine(t)
	m.install("npm", "npm ls -g --depth=0 --json", `{"dependencies":{"expo":{},"typescript":{},"react-native":{}}}`)

	result := m.run(checkMisplacedGlobals)

	assertStatus(t, result, StatusWarn)
	if result.Fix != "npm uninstall -g expo react-native" {
		t.Errorf("fix = %q", result.Fix)
	}
}

func TestOutdatedGlobalsReadsOutputOfFailingCommand(t *testing.T) {
	m := newMachine(t)
	m.install("npm", "npm outdated -g --json", `{"eas-cli":{"current":"16.28.0","latest":"24.8.0"}}`)
	m.failing("npm outdated -g --json", errors.New("exit status 1"))

	result := m.run(checkOutdatedGlobals)

	assertStatus(t, result, StatusWarn)
	assertContains(t, result.Details[0], "eas-cli 16.28.0 → 24.8.0")
}

func TestOutdatedGlobalsWhenNothingIsOutdated(t *testing.T) {
	m := newMachine(t)
	m.install("npm", "", "")

	assertStatus(t, m.run(checkOutdatedGlobals), StatusPass)
}

func TestLimitList(t *testing.T) {
	items := strings.Split("a b c d e f g h i j", " ")

	got := limitList(items)

	if len(got) != maxListedPackages+1 || got[maxListedPackages] != "and 2 more" {
		t.Errorf("limitList = %q", got)
	}
}

func TestNetworkEndpoint(t *testing.T) {
	m := newMachine(t)
	github := endpoints[0]

	m.probe.latency = 80 * time.Millisecond
	assertStatus(t, m.run(github.check), StatusPass)

	m.probe.latency = 2 * time.Second
	assertStatus(t, m.run(github.check), StatusWarn)

	m.probe.reachErr = errors.New("connection refused")
	assertStatus(t, m.run(github.check), StatusFail)
}

func TestProxyValuesAreNeverShown(t *testing.T) {
	m := newMachine(t)
	m.vars["HTTPS_PROXY"] = "http://user:secret@proxy:8080"

	result := m.run(checkProxyVariables)

	assertStatus(t, result, StatusWarn)
	if strings.Contains(result.Summary+strings.Join(result.Details, ""), "secret") {
		t.Errorf("proxy credentials leaked: %q", result.Summary)
	}
}

func TestDockerNotRunning(t *testing.T) {
	m := newMachine(t)
	m.install("docker", "", "")
	m.failing("docker info --format {{.MemTotal}}", errors.New("failed to connect to the docker API"))

	assertStatus(t, m.run(checkDocker), StatusWarn)
}

func TestJudgeBrewServices(t *testing.T) {
	result := judgeBrewServices([]brewService{{"postgresql@18", "started"}, {"redis", "error"}})

	assertStatus(t, result, StatusFail)
	if result.Fix != "brew services restart redis" {
		t.Errorf("fix = %q", result.Fix)
	}
	assertStatus(t, judgeBrewServices([]brewService{{"redis", "none"}}), StatusPass)
}

func TestFailingAgents(t *testing.T) {
	list := "PID\tStatus\tLabel\n-\t78\tcom.example.broken\n123\t0\tcom.example.running\n-\t-9\tcom.example.killed\n-\t1\tcom.apple.other"
	labels := map[string]bool{"com.example.broken": true, "com.example.running": true, "com.example.killed": true}

	failing := failingAgents(list, labels)

	if len(failing) != 1 || failing[0] != "com.example.broken (exit 78)" {
		t.Errorf("failing = %q", failing)
	}
}

func TestCacheOverLimit(t *testing.T) {
	m := newMachine(t)
	m.file("cache/blob")
	small := cache{name: "tiny", path: inHome("cache"), limit: 0}

	result := m.run(small.check)

	assertStatus(t, result, StatusWarn)
	if result.Fix != cleanFix {
		t.Errorf("fix = %q", result.Fix)
	}
	assertStatus(t, m.run(cache{name: "absent", path: inHome("nope")}.check), StatusPass)
}

func TestOlderRuntimes(t *testing.T) {
	runtimes := []macos.SimRuntime{{Version: "18.6", SizeBytes: 8}, {Version: "27.0", SizeBytes: 9}, {Version: "26.5", SizeBytes: 10}}

	names, total := olderRuntimes(runtimes, Version{27, 0, 0})

	if len(names) != 2 || total != 18 {
		t.Errorf("older = %q, total %d", names, total)
	}
}

func TestDiskSpace(t *testing.T) {
	m := newMachine(t)
	for _, tt := range []struct {
		free uint64
		want Status
	}{{10 * gigabyte, StatusFail}, {30 * gigabyte, StatusWarn}, {200 * gigabyte, StatusPass}} {
		m.probe.free = tt.free
		assertStatus(t, m.run(checkDiskSpace), tt.want)
	}
}

func TestMemoryPressure(t *testing.T) {
	m := newMachine(t)
	m.install("sysctl", "sysctl -n kern.memorystatus_vm_pressure_level", "4")
	m.runner.Outputs["sysctl -n vm.swapusage"] = "total = 2048.00M  used = 850.62M  free = 1197.38M  (encrypted)"
	m.runner.Outputs["ps -axo rss=,comm= -m"] = "674720 /Applications/Claude.app/Contents/MacOS/Claude Helper\n551376 /usr/bin/cursor"

	result := m.run(checkMemoryPressure)

	assertStatus(t, result, StatusFail)
	if len(result.Details) != 2 || !strings.HasPrefix(result.Details[0], "Claude Helper uses") {
		t.Errorf("details = %q", result.Details)
	}
}

func TestParseSwapUsed(t *testing.T) {
	if got := parseSwapUsed("total = 6144.00M  used = 4.50G  free = 1.5G"); got != int64(4.5*gigabyte) {
		t.Errorf("parseSwapUsed = %d", got)
	}
}

func TestSecuritySetting(t *testing.T) {
	m := newMachine(t)
	firewall := settingCheck{command: []string{firewallBinary, "--getglobalstate"}, enabledText: "is enabled"}
	m.runner.Outputs[firewallBinary+" --getglobalstate"] = "Firewall is disabled. (State = 0)"
	assertStatus(t, m.run(firewall.check), StatusWarn)

	m.runner.Outputs[firewallBinary+" --getglobalstate"] = "Firewall is enabled. (State = 1)"
	assertStatus(t, m.run(firewall.check), StatusPass)
}

func TestTimeMachine(t *testing.T) {
	m := newMachine(t)
	m.install("tmutil", "tmutil destinationinfo", "tmutil: No destinations configured.")
	assertStatus(t, m.run(checkTimeMachine), StatusWarn)

	m.runner.Outputs["tmutil destinationinfo"] = "Name : Backup\nKind : Local"
	m.runner.Outputs["tmutil latestbackup"] = "/Volumes/Backup/2026-09-27-101500.backup"
	assertStatus(t, m.run(checkTimeMachine), StatusPass)

	m.runner.Outputs["tmutil latestbackup"] = "/Volumes/Backup/2026-09-01-101500.backup"
	assertStatus(t, m.run(checkTimeMachine), StatusWarn)
}

func TestSoftwareUpdates(t *testing.T) {
	m := newMachine(t)
	m.runner.Outputs["defaults export "+softwareUpdatePlist+" -"] = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>RecommendedUpdates</key><array>
<dict><key>Display Name</key><string>macOS 27.0.1</string></dict></array></dict></plist>`

	result := m.run(checkSoftwareUpdates)

	assertStatus(t, result, StatusWarn)
	assertContains(t, result.Summary, "macOS 27.0.1")
}

func TestUptime(t *testing.T) {
	m := newMachine(t)
	booted := m.env().Now.Add(-20 * day).Unix()
	m.runner.Outputs["sysctl -n kern.boottime"] = "{ sec = " + strconv.FormatInt(booted, 10) + ", usec = 0 } Tue Sep  9 12:00:00 2026"

	assertStatus(t, m.run(checkUptime), StatusWarn)
}

func TestBattery(t *testing.T) {
	report := []byte(`{"SPPowerDataType":[{"_name":"spbattery_information","sppower_battery_health_info":
		{"sppower_battery_cycle_count":68,"sppower_battery_health":"Good","sppower_battery_health_maximum_capacity":"96 %"}}]}`)

	health, ok := parseBatteryHealth(report)
	if !ok {
		t.Fatal("no battery found")
	}
	result := judgeBattery(health)
	assertStatus(t, result, StatusPass)
	assertContains(t, result.Summary, "96% capacity")

	health.MaxCapacity = "74 %"
	assertStatus(t, judgeBattery(health), StatusWarn)

	if _, ok := parseBatteryHealth([]byte(`{"SPPowerDataType":[{"_name":"sppower_information"}]}`)); ok {
		t.Error("found a battery on a desktop")
	}
}

func TestThermalAndPowerMode(t *testing.T) {
	m := newMachine(t)
	m.runner.Outputs["pmset -g therm"] = "CPU_Scheduler_Limit \t= 100\nCPU_Speed_Limit \t= 70"
	m.runner.Outputs["pmset -g"] = " lowpowermode         1\n powernap             1"

	assertStatus(t, m.run(checkThermal), StatusWarn)
	assertStatus(t, m.run(checkLowPowerMode), StatusWarn)
}

func TestKernelPanics(t *testing.T) {
	m := newMachine(t)
	reports := filepath.Join(m.root, diagnosticReports)
	if err := os.MkdirAll(reports, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"panic-full-2026-09-20-101010.panic", "Safari-2026-09-20.ips"} {
		if err := os.WriteFile(filepath.Join(reports, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result := m.run(checkKernelPanics)

	assertStatus(t, result, StatusWarn)
	assertContains(t, result.Summary, "1 kernel panic")
}

func TestParseClockOffset(t *testing.T) {
	offset, ok := parseClockOffset("sntp_exchange {\n  result: 6 (Timeout)\n}\n-3.250000 +/- 0.014937 time.apple.com 17.253.38.35")
	if !ok || offset != -3.25 {
		t.Errorf("offset = %v, %v", offset, ok)
	}
	if _, ok := parseClockOffset("sntp: Exchange failed: Timeout"); ok {
		t.Error("parsed an offset from a failure")
	}
}
