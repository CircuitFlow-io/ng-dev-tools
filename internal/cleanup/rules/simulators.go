package rules

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/fsx"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

const simRuntimePrefix = "com.apple.CoreSimulator.SimRuntime."

// simulatorsRule finds simulators whose runtime is gone and ones not booted recently.
type simulatorsRule struct{}

func (simulatorsRule) Name() string               { return "iOS simulators" }
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
	deviceDir := filepath.Dir(sim.DataPath)
	lastUsed := sim.LastUsedAt
	if lastUsed.IsZero() {
		lastUsed = fsx.ModTime(deviceDir)
	}

	title, safety := "", cleanup.SafetyReview
	switch {
	case !sim.IsAvailable:
		title, safety = "Unavailable simulator", cleanup.SafetySafe
	case env.IsStale(lastUsed):
		title = "Unused simulator"
	default:
		return cleanup.Item{}, false
	}

	item := cleanup.PathItem(dev, safety, fmt.Sprintf("%s: %s (%s)", title, sim.Name, runtimeName(sim.RuntimeID)), deviceDir)
	item.LastUsed = lastUsed
	item.RemoveCommand = []string{"xcrun", "simctl", "delete", sim.UDID}
	return item, true
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

// runtimeName turns "com.apple.CoreSimulator.SimRuntime.iOS-18-6" into "iOS 18.6".
func runtimeName(identifier string) string {
	name := strings.TrimPrefix(identifier, simRuntimePrefix)
	platform, version, found := strings.Cut(name, "-")
	if !found {
		return name
	}
	return platform + " " + strings.ReplaceAll(version, "-", ".")
}
