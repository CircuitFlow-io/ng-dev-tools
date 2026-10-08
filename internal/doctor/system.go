package doctor

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	diskWarnFree     = 50 * gigabyte
	diskFailFree     = 20 * gigabyte
	minMemory        = 16 * gigabyte
	swapWarnUsed     = 4 * gigabyte
	topProcessCount  = 3
	pressureWarning  = 2
	pressureCritical = 4
	firewallBinary   = "/usr/libexec/ApplicationFirewall/socketfilterfw"
)

var swapUsedPattern = regexp.MustCompile(`used = ([\d.]+)([KMG])`)

var unitBytes = map[string]float64{"K": 1 << 10, "M": 1 << 20, "G": 1 << 30}

func systemChecks() []Check {
	return append([]Check{
		{Name: "Free disk space", Group: GroupSystem, Run: checkDiskSpace},
		{Name: "Installed memory", Group: GroupSystem, Run: checkInstalledMemory},
		{Name: "Memory pressure", Group: GroupSystem, Run: checkMemoryPressure},
		{Name: "FileVault", Group: GroupSystem, Run: settingCheck{
			command: []string{"fdesetup", "status"}, enabledText: "FileVault is On",
			problem: "disk encryption is off", fix: "System Settings > Privacy & Security > FileVault",
		}.check},
		{Name: "Firewall", Group: GroupSystem, Run: settingCheck{
			command: []string{firewallBinary, "--getglobalstate"}, enabledText: "is enabled",
			problem: "the firewall is off", fix: "System Settings > Network > Firewall",
		}.check},
		{Name: "System Integrity Protection", Group: GroupSystem, Run: settingCheck{
			command: []string{"csrutil", "status"}, enabledText: "status: enabled",
			problem: "SIP is disabled", fix: "boot into Recovery and run: csrutil enable",
		}.check},
		{Name: "macOS updates", Group: GroupSystem, Run: checkSoftwareUpdates},
	}, wellnessChecks()...)
}

func checkDiskSpace(_ context.Context, env Env) Result {
	free, err := env.Probe.DiskFree(env.systemPath("/"))
	if err != nil {
		return warn("could not read free space", "").with(err.Error())
	}
	summary := ui.Bytes(int64(free)) + " free"
	switch {
	case free < diskFailFree:
		return fail(summary+", Xcode and simulator updates will fail", cleanFix)
	case free < diskWarnFree:
		return warn(summary+", getting low", cleanFix)
	default:
		return pass(summary)
	}
}

func (e Env) sysctl(ctx context.Context, name string) (string, error) {
	return e.output(ctx, "sysctl", "-n", name)
}

func checkInstalledMemory(ctx context.Context, env Env) Result {
	out, err := env.sysctl(ctx, "hw.memsize")
	if err != nil {
		return skip("could not read installed memory")
	}
	memory, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		return skip("could not read installed memory")
	}
	summary := fmt.Sprintf("%d GB", memory/gigabyte)
	if memory < minMemory {
		return warn(summary+", simulators and emulators will swap", "close simulators and emulators you are not using")
	}
	return pass(summary)
}

func checkMemoryPressure(ctx context.Context, env Env) Result {
	level, err := env.sysctl(ctx, "kern.memorystatus_vm_pressure_level")
	if err != nil {
		return skip("could not read memory pressure")
	}
	swap := env.swapUsed(ctx)
	summary := fmt.Sprintf("%s pressure, %s swap used", pressureName(level), ui.Bytes(swap))
	var result Result
	switch {
	case level == strconv.Itoa(pressureCritical):
		result = fail(summary, "quit memory-hungry apps or restart")
	case level == strconv.Itoa(pressureWarning), swap > swapWarnUsed:
		result = warn(summary, "quit memory-hungry apps or restart")
	default:
		return pass(summary)
	}
	return result.with(env.topMemoryUsers(ctx)...)
}

func pressureName(level string) string {
	switch level {
	case strconv.Itoa(pressureCritical):
		return "critical"
	case strconv.Itoa(pressureWarning):
		return "high"
	default:
		return "normal"
	}
}

func (e Env) swapUsed(ctx context.Context) int64 {
	out, err := e.sysctl(ctx, "vm.swapusage")
	if err != nil {
		return 0
	}
	return parseSwapUsed(out)
}

func parseSwapUsed(swapusage string) int64 {
	match := swapUsedPattern.FindStringSubmatch(swapusage)
	if match == nil {
		return 0
	}
	amount, _ := strconv.ParseFloat(match[1], 64)
	return int64(amount * unitBytes[match[2]])
}

// topMemoryUsers lists the processes using the most memory, as "name  size".
func (e Env) topMemoryUsers(ctx context.Context) []string {
	out, err := e.output(ctx, "ps", "-axo", "rss=,comm=", "-m")
	if err != nil {
		return nil
	}
	var users []string
	for _, line := range strings.Split(out, "\n") {
		rss, command, ok := strings.Cut(strings.TrimSpace(line), " ")
		kilobytes, err := strconv.ParseInt(rss, 10, 64)
		if !ok || err != nil {
			continue
		}
		users = append(users, fmt.Sprintf("%s uses %s", filepath.Base(strings.TrimSpace(command)), ui.Bytes(kilobytes<<10)))
		if len(users) == topProcessCount {
			break
		}
	}
	return users
}

// settingCheck reads an on/off security setting from a command's output.
type settingCheck struct {
	command     []string
	enabledText string
	problem     string
	fix         string
}

func (s settingCheck) check(ctx context.Context, env Env) Result {
	out, err := env.output(ctx, s.command[0], s.command[1:]...)
	if err != nil {
		return skip("could not read the setting")
	}
	if !strings.Contains(out, s.enabledText) {
		return warn(s.problem, s.fix)
	}
	return pass("on")
}
