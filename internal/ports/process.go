// Package ports finds processes listening on TCP ports and stops them.
package ports

import (
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"
)

// systemExecutableDirs hold macOS's own daemons, which developers rarely mean to stop.
var systemExecutableDirs = []string{"/System/", "/usr/libexec/", "/usr/sbin/", "/sbin/"}

const (
	dockerBackendMarker    = "/Docker.app/"
	simulatorRuntimeMarker = ".simruntime/"
	airPlayReceiverName    = "ControlCenter"
)

// kind groups processes that need a word of caution before being stopped.
type kind int

const (
	kindRegular kind = iota
	kindSystem
	kindSimulator
	kindAirPlay
	kindDocker
)

var kindTags = map[kind]string{
	kindSystem:    "system",
	kindSimulator: "simulator",
	kindAirPlay:   "airplay",
	kindDocker:    "docker",
}

var kindCautions = map[kind]string{
	kindSystem:    "This is a macOS system process.",
	kindSimulator: "This runs inside a booted iOS Simulator; shut the simulator down instead.",
	kindAirPlay:   "AirPlay Receiver holds ports 5000 and 7000 and restarts on its own. Turn it off in System Settings › General › AirDrop & Handoff.",
	kindDocker:    "Docker Desktop publishes container ports; stopping it stops every container. Prefer `docker stop`.",
}

// Process is a process with at least one listening TCP socket.
type Process struct {
	PID         int
	Name        string
	Executable  string
	CommandLine string
	StartedAt   time.Time
	WorkDir     string
	// Project is the git repository containing WorkDir, or WorkDir itself outside a repository.
	Project string
	// Addresses are the listening sockets, such as "*:3000" or "127.0.0.1:5432".
	Addresses []string
	Ports     []int
}

// IsSystem reports whether the process belongs to macOS or a simulator rather than to the user.
func (p Process) IsSystem() bool {
	switch p.kind() {
	case kindSystem, kindSimulator, kindAirPlay:
		return true
	default:
		return false
	}
}

// Caution explains why stopping the process may not be what the user wants, or is empty.
func (p Process) Caution() string {
	return kindCautions[p.kind()]
}

// Tag is a one-word label for Caution, or empty.
func (p Process) Tag() string {
	return kindTags[p.kind()]
}

func (p Process) kind() kind {
	switch {
	case strings.Contains(p.Executable, dockerBackendMarker):
		return kindDocker
	case strings.Contains(p.Executable, simulatorRuntimeMarker):
		return kindSimulator
	case p.Name == airPlayReceiverName:
		return kindAirPlay
	case slices.ContainsFunc(systemExecutableDirs, func(dir string) bool { return strings.HasPrefix(p.Executable, dir) }):
		return kindSystem
	default:
		return kindRegular
	}
}

// ListensOn reports whether the process holds port.
func (p Process) ListensOn(port int) bool {
	return slices.Contains(p.Ports, port)
}

// IsExposed reports whether any socket accepts connections from other machines.
func (p Process) IsExposed() bool {
	for _, address := range p.Addresses {
		host, _, err := net.SplitHostPort(address)
		if err != nil || host == "*" {
			return true
		}
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			return true
		}
	}
	return false
}

// PortList formats the ports like ":3000 :9229".
func (p Process) PortList() string {
	parts := make([]string, len(p.Ports))
	for i, port := range p.Ports {
		parts[i] = ":" + strconv.Itoa(port)
	}
	return strings.Join(parts, " ")
}

// Label names the process for messages, like "node (pid 123) on :3000".
func (p Process) Label() string {
	return fmt.Sprintf("%s (pid %d) on %s", p.Name, p.PID, p.PortList())
}

// Uptime is how long the process has been running, or zero when unknown.
func (p Process) Uptime(now time.Time) time.Duration {
	if p.StartedAt.IsZero() {
		return 0
	}
	return now.Sub(p.StartedAt)
}

// Holding returns the processes listening on any of wanted, and the wanted ports nobody holds.
func Holding(processes []Process, wanted []int) (holders []Process, free []int) {
	for _, port := range wanted {
		found := false
		for _, p := range processes {
			if p.ListensOn(port) {
				found = true
				if !slices.ContainsFunc(holders, func(h Process) bool { return h.PID == p.PID }) {
					holders = append(holders, p)
				}
			}
		}
		if !found {
			free = append(free, port)
		}
	}
	return holders, free
}
