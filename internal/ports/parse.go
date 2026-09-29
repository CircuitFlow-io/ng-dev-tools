package ports

import (
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	day            = 24 * time.Hour
	secondsPerUnit = 60
)

type lsofRecord struct {
	name      string
	addresses []string
}

// parseListeners reads `lsof -F pcn` output: a "p" line starts each process, followed by its
// "c" command name and one "n" line per socket. IPv4 and IPv6 sockets on the same port collapse.
func parseListeners(output string) []Process {
	records := map[int]*lsofRecord{}
	var order []int
	var current *lsofRecord
	for line := range strings.Lines(output) {
		line = strings.TrimRight(line, "\n")
		if line == "" {
			continue
		}
		field, value := line[0], line[1:]
		switch field {
		case 'p':
			pid, err := strconv.Atoi(value)
			if err != nil {
				current = nil
				continue
			}
			if records[pid] == nil {
				records[pid] = &lsofRecord{}
				order = append(order, pid)
			}
			current = records[pid]
		case 'c':
			if current != nil {
				current.name = value
			}
		case 'n':
			if current != nil && !slices.Contains(current.addresses, value) {
				current.addresses = append(current.addresses, value)
			}
		}
	}

	processes := make([]Process, 0, len(order))
	for _, pid := range order {
		record := records[pid]
		processes = append(processes, Process{
			PID:       pid,
			Name:      record.name,
			Addresses: record.addresses,
			Ports:     portsOf(record.addresses),
		})
	}
	return processes
}

func portsOf(addresses []string) []int {
	var ports []int
	for _, address := range addresses {
		_, portText, err := net.SplitHostPort(address)
		if err != nil {
			continue
		}
		port, err := strconv.Atoi(portText)
		if err != nil || slices.Contains(ports, port) {
			continue
		}
		ports = append(ports, port)
	}
	slices.Sort(ports)
	return ports
}

// parseWorkDirs reads `lsof -a -d cwd -F pn` output into a pid to directory map.
func parseWorkDirs(output string) map[int]string {
	dirs := map[int]string{}
	pid := 0
	for line := range strings.Lines(output) {
		line = strings.TrimRight(line, "\n")
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ = strconv.Atoi(line[1:])
		case 'n':
			if pid > 0 {
				dirs[pid] = line[1:]
			}
		}
	}
	return dirs
}

// processInfo is what `ps -o pid=,etime=,comm=` reports for one process.
type processInfo struct {
	elapsed    time.Duration
	executable string
}

// parseProcessInfo reads lines of "pid elapsed executable"; the executable may contain spaces.
func parseProcessInfo(output string) map[int]processInfo {
	infos := map[int]processInfo{}
	for line := range strings.Lines(output) {
		pidText, rest, ok := cutField(line)
		if !ok {
			continue
		}
		elapsedText, executable, ok := cutField(rest)
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(pidText)
		if err != nil {
			continue
		}
		elapsed, err := parseElapsed(elapsedText)
		if err != nil {
			continue
		}
		infos[pid] = processInfo{elapsed: elapsed, executable: strings.TrimSpace(executable)}
	}
	return infos
}

// parseCommandLines reads lines of "pid arguments".
func parseCommandLines(output string) map[int]string {
	lines := map[int]string{}
	for line := range strings.Lines(output) {
		pidText, args, ok := cutField(line)
		if !ok {
			continue
		}
		if pid, err := strconv.Atoi(pidText); err == nil {
			lines[pid] = strings.TrimSpace(args)
		}
	}
	return lines
}

// cutField splits off the first whitespace-separated field.
func cutField(s string) (field, rest string, ok bool) {
	s = strings.TrimLeft(s, " \t")
	end := strings.IndexAny(s, " \t")
	if end <= 0 {
		return "", "", false
	}
	return s[:end], strings.TrimLeft(s[end:], " \t"), true
}

// parseElapsed reads ps's etime format: [[days-]hours:]minutes:seconds.
func parseElapsed(value string) (time.Duration, error) {
	var days int
	if dayText, rest, found := strings.Cut(value, "-"); found {
		n, err := strconv.Atoi(dayText)
		if err != nil {
			return 0, fmt.Errorf("elapsed time %q: %w", value, err)
		}
		days, value = n, rest
	}
	parts := strings.Split(value, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("elapsed time %q: unexpected format", value)
	}
	var seconds int
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return 0, fmt.Errorf("elapsed time %q: %w", value, err)
		}
		seconds = seconds*secondsPerUnit + n
	}
	return time.Duration(days)*day + time.Duration(seconds)*time.Second, nil
}
