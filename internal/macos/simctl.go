package macos

import (
	"context"
	"encoding/json"
	"time"
)

// Simulator is a CoreSimulator device as reported by `xcrun simctl list devices -j`.
type Simulator struct {
	UDID        string    `json:"udid"`
	Name        string    `json:"name"`
	DataPath    string    `json:"dataPath"`
	IsAvailable bool      `json:"isAvailable"`
	LastUsedAt  time.Time `json:"lastUsedAt"`
	RuntimeID   string    `json:"-"`
}

// SimRuntime is a simulator runtime image as reported by `xcrun simctl runtime list -j`.
type SimRuntime struct {
	Identifier        string    `json:"identifier"`
	RuntimeIdentifier string    `json:"runtimeIdentifier"`
	Version           string    `json:"version"`
	SizeBytes         int64     `json:"sizeBytes"`
	Deletable         bool      `json:"deletable"`
	LastUsedAt        time.Time `json:"lastUsedAt"`
}

// Simulators lists all simulator devices.
func Simulators(ctx context.Context, r Runner) ([]Simulator, error) {
	out, err := r.Run(ctx, "xcrun", "simctl", "list", "devices", "-j")
	if err != nil {
		return nil, err
	}
	return ParseSimulators(out)
}

// ParseSimulators decodes `simctl list devices -j` output.
func ParseSimulators(data []byte) ([]Simulator, error) {
	var listing struct {
		Devices map[string][]Simulator `json:"devices"`
	}
	if err := json.Unmarshal(data, &listing); err != nil {
		return nil, err
	}
	var sims []Simulator
	for runtimeID, devices := range listing.Devices {
		for _, device := range devices {
			device.RuntimeID = runtimeID
			sims = append(sims, device)
		}
	}
	return sims, nil
}

// SimRuntimes lists installed simulator runtimes.
func SimRuntimes(ctx context.Context, r Runner) ([]SimRuntime, error) {
	out, err := r.Run(ctx, "xcrun", "simctl", "runtime", "list", "-j")
	if err != nil {
		return nil, err
	}
	return ParseSimRuntimes(out)
}

// ParseSimRuntimes decodes `simctl runtime list -j` output.
func ParseSimRuntimes(data []byte) ([]SimRuntime, error) {
	var listing map[string]SimRuntime
	if err := json.Unmarshal(data, &listing); err != nil {
		return nil, err
	}
	runtimes := make([]SimRuntime, 0, len(listing))
	for _, rt := range listing {
		runtimes = append(runtimes, rt)
	}
	return runtimes, nil
}
