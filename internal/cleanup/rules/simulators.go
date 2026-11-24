package rules

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

const simRuntimePrefix = "com.apple.CoreSimulator.SimRuntime."

const simulatorsGroup = "Simulators"

// simulatorsRule finds iPhone, iPad, Apple Watch, Apple TV and Vision simulators whose runtime is
// gone, that were never booted, or that have sat idle past the staleness window.
type simulatorsRule struct{}

func (simulatorsRule) Name() string               { return "Simulators" }
func (simulatorsRule) Category() cleanup.Category { return dev }

func (simulatorsRule) Scan(ctx context.Context, env cleanup.Env) ([]cleanup.Item, error) {
	if !env.Runner.Available("xcrun") {
		return nil, nil
	}
	sims, err := macos.Simulators(ctx, env.Runner)
	if err != nil {
		return nil, err
	}
	var items []cleanup.Item
	for _, sim := range sims {
		if item, ok := simulatorItem(env, sim); ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func simulatorItem(env cleanup.Env, sim macos.Simulator) (cleanup.Item, bool) {
	title, safety, ok := classifySimulator(env, sim)
	if !ok {
		return cleanup.Item{}, false
	}
	item := cleanup.PathItem(dev, safety, fmt.Sprintf("%s: %s (%s)", title, sim.Name, runtimeName(sim.RuntimeID)), filepath.Dir(sim.DataPath))
	item.LastUsed = sim.LastUsedAt
	item.RemoveCommand = []string{"xcrun", "simctl", "delete", sim.UDID}
	item.Group = simulatorsGroup
	item.ShowWhenSmall = true
	return item, true
}

// classifySimulator decides why a simulator is worth removing. Device folder times are not used:
// CoreSimulator touches every device folder whenever it starts.
func classifySimulator(env cleanup.Env, sim macos.Simulator) (string, cleanup.Safety, bool) {
	switch {
	case sim.IsBooted():
		return "", 0, false
	case !sim.IsAvailable:
		return "Unavailable simulator", cleanup.SafetySafe, true
	case sim.LastUsedAt.IsZero():
		return "Never-booted simulator", cleanup.SafetyReview, true
	case env.IsStale(sim.LastUsedAt):
		return "Idle simulator", cleanup.SafetyReview, true
	}
	return "", 0, false
}

// simRuntimesRule finds simulator runtime images that have not been used recently.
type simRuntimesRule struct{}

func (simRuntimesRule) Name() string               { return "Simulator runtimes" }
func (simRuntimesRule) Category() cleanup.Category { return dev }

func (simRuntimesRule) Scan(ctx context.Context, env cleanup.Env) ([]cleanup.Item, error) {
	if !env.Runner.Available("xcrun") {
		return nil, nil
	}
	runtimes, err := macos.SimRuntimes(ctx, env.Runner)
	if err != nil {
		return nil, err
	}
	var items []cleanup.Item
	for _, rt := range runtimes {
		if !rt.Deletable || !env.IsStale(rt.LastUsedAt) {
			continue
		}
		items = append(items, cleanup.Item{
			Title:         "Simulator runtime: " + runtimeName(rt.RuntimeIdentifier),
			Detail:        fmt.Sprintf("build %s, removed with simctl", rt.Identifier),
			Category:      dev,
			Safety:        cleanup.SafetyReview,
			RemoveCommand: []string{"xcrun", "simctl", "runtime", "delete", rt.Identifier},
			Size:          rt.SizeBytes,
			LastUsed:      rt.LastUsedAt,
		})
	}
	return items, nil
}

// runtimeName turns "com.apple.CoreSimulator.SimRuntime.iOS-18-6" into "iOS 18.6". Device-specific
// runtimes such as "iOS-27-2-iPhone19-4" keep only their version: "iOS 27.2".
func runtimeName(identifier string) string {
	name := strings.TrimPrefix(identifier, simRuntimePrefix)
	platform, version, found := strings.Cut(name, "-")
	if !found {
		return name
	}
	parts := strings.Split(version, "-")
	numeric := parts[:0]
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			break
		}
		numeric = append(numeric, part)
	}
	if len(numeric) == 0 {
		return platform + " " + strings.Join(parts, ".")
	}
	return platform + " " + strings.Join(numeric, ".")
}
