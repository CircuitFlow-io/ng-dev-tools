package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"howett.net/plist"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	day                 = 24 * time.Hour
	maxBackupAge        = 7 * day
	maxUptime           = 14 * day
	panicWindow         = 30 * day
	maxBatteryCycles    = 1000
	minBatteryCapacity  = 80
	fullCPUSpeed        = 100
	maxClockOffset      = 2.0
	lowPowerModeOn      = "1"
	backupTimeLayout    = "2006-01-02-150405"
	softwareUpdatePlist = "/Library/Preferences/com.apple.SoftwareUpdate"
	diagnosticReports   = "/Library/Logs/DiagnosticReports"
	timeServer          = "time.apple.com"
)

var (
	backupTimePattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2}-\d{6}`)
	bootTimePattern   = regexp.MustCompile(`sec = (\d+)`)
	cpuLimitPattern   = regexp.MustCompile(`CPU_Speed_Limit\s*=\s*(\d+)`)
	healthyBattery    = map[string]bool{"Good": true, "Normal": true}
)

func wellnessChecks() []Check {
	return []Check{
		{Name: "Uptime", Group: GroupSystem, Run: checkUptime},
		{Name: "Thermal throttling", Group: GroupSystem, Run: checkThermal},
		{Name: "Battery", Group: GroupSystem, Run: checkBattery},
		{Name: "Low Power Mode", Group: GroupSystem, Run: checkLowPowerMode},
		{Name: "Kernel panics", Group: GroupSystem, Run: checkKernelPanics},
		{Name: "Clock", Group: GroupSystem, NeedsNetwork: true, Run: checkClock},
	}
}

func checkTimeMachine(ctx context.Context, env Env) Result {
	destinations, err := env.output(ctx, "tmutil", "destinationinfo")
	if err != nil {
		return skip("could not read Time Machine settings")
	}
	if strings.Contains(destinations, "No destinations") {
		return warn("not set up", "System Settings > General > Time Machine")
	}
	latest, _ := env.output(ctx, "tmutil", "latestbackup")
	backedUp, ok := parseBackupTime(latest)
	if !ok {
		return warn("no completed backup found", "connect your backup disk and run: tmutil startbackup")
	}
	age := env.Now.Sub(backedUp)
	summary := "last backup " + ui.Age(env.Now, backedUp) + " ago"
	if age > maxBackupAge {
		return warn(summary, "connect your backup disk and run: tmutil startbackup")
	}
	return pass(summary)
}

func parseBackupTime(path string) (time.Time, bool) {
	stamp := backupTimePattern.FindString(path)
	if stamp == "" {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation(backupTimeLayout, stamp, time.Local)
	return t, err == nil
}

func checkSoftwareUpdates(ctx context.Context, env Env) Result {
	out, err := env.Runner.Run(ctx, "defaults", "export", softwareUpdatePlist, "-")
	if err != nil {
		return skip("could not read Software Update status")
	}
	var prefs struct {
		RecommendedUpdates []struct {
			DisplayName string `plist:"Display Name"`
		} `plist:"RecommendedUpdates"`
	}
	if _, err := plist.Unmarshal(out, &prefs); err != nil {
		return skip("could not read Software Update status")
	}
	if len(prefs.RecommendedUpdates) == 0 {
		return pass("up to date as of the last background check")
	}
	names := make([]string, len(prefs.RecommendedUpdates))
	for i, update := range prefs.RecommendedUpdates {
		names[i] = update.DisplayName
	}
	return warn(strings.Join(names, ", ")+" ready to install", "System Settings > General > Software Update")
}

func checkUptime(ctx context.Context, env Env) Result {
	out, err := env.sysctl(ctx, "kern.boottime")
	match := bootTimePattern.FindStringSubmatch(out)
	if err != nil || match == nil {
		return skip("could not read the boot time")
	}
	seconds, _ := strconv.ParseInt(match[1], 10, 64)
	up := env.Now.Sub(time.Unix(seconds, 0))
	summary := "up " + ui.Elapsed(up)
	if up > maxUptime {
		return warn(summary, "restart to install updates and release leaked memory")
	}
	return pass(summary)
}

func checkThermal(ctx context.Context, env Env) Result {
	out, err := env.output(ctx, "pmset", "-g", "therm")
	if err != nil {
		return skip("could not read thermal state")
	}
	match := cpuLimitPattern.FindStringSubmatch(out)
	if match == nil {
		return pass("not throttled")
	}
	if limit, _ := strconv.Atoi(match[1]); limit < fullCPUSpeed {
		return warn(fmt.Sprintf("CPU limited to %d%% by heat", limit), "give the Mac airflow and close heavy background work")
	}
	return pass("not throttled")
}

type batteryHealth struct {
	CycleCount  int    `json:"sppower_battery_cycle_count"`
	Condition   string `json:"sppower_battery_health"`
	MaxCapacity string `json:"sppower_battery_health_maximum_capacity"`
}

func checkBattery(ctx context.Context, env Env) Result {
	out, err := env.Runner.Run(ctx, "system_profiler", "SPPowerDataType", "-json")
	if err != nil {
		return skip("could not read battery information")
	}
	health, ok := parseBatteryHealth(out)
	if !ok {
		return skip("no battery")
	}
	return judgeBattery(health)
}

func parseBatteryHealth(data []byte) (batteryHealth, bool) {
	var report struct {
		Power []struct {
			Health *batteryHealth `json:"sppower_battery_health_info"`
		} `json:"SPPowerDataType"`
	}
	if json.Unmarshal(data, &report) != nil {
		return batteryHealth{}, false
	}
	for _, section := range report.Power {
		if section.Health != nil {
			return *section.Health, true
		}
	}
	return batteryHealth{}, false
}

func judgeBattery(h batteryHealth) Result {
	capacity, _ := ParseVersion(h.MaxCapacity)
	summary := fmt.Sprintf("%s, %s, %d%% capacity", h.Condition, ui.Count(h.CycleCount, "cycle"), capacity.Major)
	if !healthyBattery[h.Condition] || h.CycleCount > maxBatteryCycles || capacity.Major < minBatteryCapacity {
		return warn(summary, "System Settings > Battery > Battery Health")
	}
	return pass(summary)
}

func checkLowPowerMode(ctx context.Context, env Env) Result {
	out, err := env.output(ctx, "pmset", "-g")
	if err != nil {
		return skip("could not read power settings")
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && (fields[0] == "lowpowermode" || fields[0] == "powermode") && fields[1] == lowPowerModeOn {
			return warn("on, builds run slower", "System Settings > Battery > Low Power Mode")
		}
	}
	return pass("off")
}

func checkKernelPanics(_ context.Context, env Env) Result {
	entries, err := os.ReadDir(env.systemPath(diagnosticReports))
	if err != nil {
		return skip("could not read crash reports")
	}
	var recent int
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !isPanicReport(entry.Name()) {
			continue
		}
		if env.Now.Sub(info.ModTime()) < panicWindow {
			recent++
		}
	}
	if recent > 0 {
		return warn(ui.Count(recent, "kernel panic")+" in the last 30 days", "Console > Crash Reports; repeated panics usually mean hardware or a driver")
	}
	return pass("none in the last 30 days")
}

func isPanicReport(name string) bool {
	return strings.HasPrefix(name, "panic") || strings.HasSuffix(name, ".panic")
}

func checkClock(ctx context.Context, env Env) Result {
	out, err := env.output(ctx, "sntp", "-t", "3", timeServer)
	if err != nil {
		return skip("could not reach " + timeServer)
	}
	offset, ok := parseClockOffset(out)
	if !ok {
		return skip("no answer from " + timeServer)
	}
	summary := fmt.Sprintf("%.2fs off %s", math.Abs(offset), timeServer)
	if math.Abs(offset) > maxClockOffset {
		return warn(summary, "System Settings > General > Date & Time > Set time and date automatically")
	}
	return pass(summary)
}

// parseClockOffset finds sntp's "+0.0029 +/- 0.0149 host address" line; failed exchanges print
// debug blocks around it.
func parseClockOffset(out string) (float64, bool) {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[1] != "+/-" {
			continue
		}
		offset, err := strconv.ParseFloat(fields[0], 64)
		return offset, err == nil
	}
	return 0, false
}
