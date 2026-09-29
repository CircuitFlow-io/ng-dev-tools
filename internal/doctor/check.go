// Package doctor checks that this Mac is ready for fullstack, mobile and Go development.
// Every check only reads state; failing checks suggest the command that fixes them.
package doctor

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// Status is how a check went.
type Status int

const (
	StatusPass Status = iota
	StatusWarn
	StatusFail
	StatusSkip
)

// Result is what a check found.
type Result struct {
	Status  Status
	Summary string
	Details []string
	// Fix is the command or setting that resolves a warning or failure.
	Fix string
}

func pass(summary string) Result {
	return Result{Status: StatusPass, Summary: summary}
}

func warn(summary, fix string) Result {
	return Result{Status: StatusWarn, Summary: summary, Fix: fix}
}

func fail(summary, fix string) Result {
	return Result{Status: StatusFail, Summary: summary, Fix: fix}
}

func skip(summary string) Result {
	return Result{Status: StatusSkip, Summary: summary}
}

func (r Result) with(details ...string) Result {
	r.Details = append(slices.Clone(r.Details), details...)
	return r
}

// IsProblem reports whether the result needs attention.
func (r Result) IsProblem() bool {
	return r.Status == StatusWarn || r.Status == StatusFail
}

// Check is one thing doctor verifies.
type Check struct {
	Name  string
	Group Group
	// NeedsNetwork checks are skipped in offline mode.
	NeedsNetwork bool
	Run          func(ctx context.Context, env Env) Result
}

// Group is a section of the report.
type Group int

const (
	GroupNode Group = iota
	GroupIOS
	GroupAndroid
	GroupGo
	GroupClaude
	GroupShell
	GroupGit
	GroupGlobals
	GroupNetwork
	GroupServices
	GroupCaches
	GroupSystem
)

var groupKeys = []string{"node", "ios", "android", "go", "claude", "shell", "git", "globals", "network", "services", "caches", "system"}

var groupTitles = []string{
	"Node.js & JavaScript",
	"iOS, React Native & Expo",
	"Android",
	"Go",
	"Claude Code",
	"Shell & environment",
	"Git, GitHub & SSH",
	"Global packages",
	"Network",
	"Background services",
	"Build caches",
	"System",
}

// Key is the name used to pick the group on the command line.
func (g Group) Key() string {
	return groupKeys[g]
}

// Title is the group's heading in the report.
func (g Group) Title() string {
	return groupTitles[g]
}

// GroupKeys lists every group's command-line name in report order.
func GroupKeys() []string {
	return slices.Clone(groupKeys)
}

// ParseGroup finds the group with the given command-line name.
func ParseGroup(key string) (Group, error) {
	i := slices.Index(groupKeys, strings.ToLower(key))
	if i < 0 {
		return 0, fmt.Errorf("unknown group %q (valid: %s)", key, strings.Join(groupKeys, ", "))
	}
	return Group(i), nil
}
