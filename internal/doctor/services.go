package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"howett.net/plist"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	minDockerMemory     = 4 << 30
	brewServiceError    = "error"
	brewServiceStarted  = "started"
	launchctlNotRunning = "-"
)

func servicesChecks() []Check {
	return []Check{
		{Name: "Docker", Group: GroupServices, Run: checkDocker},
		{Name: "Homebrew services", Group: GroupServices, Run: checkBrewServices},
		{Name: "Launch agents", Group: GroupServices, Run: checkLaunchAgents},
	}
}

func checkDocker(ctx context.Context, env Env) Result {
	if !env.installed("docker") {
		return skip("Docker is not installed")
	}
	out, err := env.output(ctx, "docker", "info", "--format", "{{.MemTotal}}")
	if err != nil {
		return warn("the Docker daemon is not running", "open -a Docker")
	}
	memory, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		return pass("running")
	}
	if memory < minDockerMemory {
		return warn(fmt.Sprintf("running with only %s of memory", ui.Bytes(memory)), "Docker Desktop > Settings > Resources > Memory")
	}
	return pass(fmt.Sprintf("running, %s of memory", ui.Bytes(memory)))
}

type brewService struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func checkBrewServices(ctx context.Context, env Env) Result {
	if !env.installed("brew") {
		return skip("Homebrew is not installed")
	}
	out, err := env.output(ctx, "brew", "services", "list", "--json")
	if err != nil {
		return warn("could not list services", "").with(errorLine(err))
	}
	var services []brewService
	if err := json.Unmarshal([]byte(out), &services); err != nil {
		return warn("could not read the service list", "").with(err.Error())
	}
	return judgeBrewServices(services)
}

func judgeBrewServices(services []brewService) Result {
	var failed, running []string
	for _, s := range services {
		switch s.Status {
		case brewServiceError:
			failed = append(failed, s.Name)
		case brewServiceStarted:
			running = append(running, s.Name)
		}
	}
	if len(failed) > 0 {
		return fail(strings.Join(failed, ", ")+" crashed", "brew services restart "+failed[0]).with(runningNote(running)...)
	}
	return pass(fmt.Sprintf("%s, %d running", ui.Count(len(services), "service"), len(running))).with(runningNote(running)...)
}

func runningNote(running []string) []string {
	if len(running) == 0 {
		return nil
	}
	return []string{"running: " + strings.Join(running, ", ")}
}

func checkLaunchAgents(ctx context.Context, env Env) Result {
	labels := env.launchAgentLabels()
	if len(labels) == 0 {
		return pass("none installed")
	}
	out, err := env.output(ctx, "launchctl", "list")
	if err != nil {
		return warn("could not list launch agents", "").with(errorLine(err))
	}
	failing := failingAgents(out, labels)
	if len(failing) > 0 {
		return warn(ui.Count(len(failing), "agent")+" keep failing", "launchctl print gui/$(id -u)/<label> shows why; remove the plist if the app is gone").
			with(failing...)
	}
	return pass(ui.Count(len(labels), "agent"))
}

// launchAgentLabels reads the Label of every plist in ~/Library/LaunchAgents.
func (e Env) launchAgentLabels() map[string]bool {
	files, _ := filepath.Glob(e.homePath("Library", "LaunchAgents", "*.plist"))
	labels := map[string]bool{}
	for _, file := range files {
		if label := plistLabel(file); label != "" {
			labels[label] = true
		}
	}
	return labels
}

func plistLabel(file string) string {
	data, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	var agent struct {
		Label string `plist:"Label"`
	}
	if _, err := plist.Unmarshal(data, &agent); err != nil {
		return ""
	}
	return agent.Label
}

// failingAgents finds the user's agents that are not running and last exited with an error code.
// Negative statuses are deaths by signal, usually macOS reclaiming memory, so they are ignored.
func failingAgents(launchctlList string, labels map[string]bool) []string {
	var failing []string
	for _, line := range strings.Split(launchctlList, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || !labels[fields[2]] || fields[0] != launchctlNotRunning {
			continue
		}
		if status, err := strconv.Atoi(fields[1]); err == nil && status > 0 {
			failing = append(failing, fmt.Sprintf("%s (exit %d)", fields[2], status))
		}
	}
	return failing
}
