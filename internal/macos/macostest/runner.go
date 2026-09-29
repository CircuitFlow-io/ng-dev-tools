// Package macostest provides a scripted macos.Runner for tests.
package macostest

import (
	"context"
	"strings"
	"sync"
)

// Runner returns canned output keyed by the full command line and records every call.
type Runner struct {
	Outputs map[string]string
	Errors  map[string]error
	Tools   map[string]bool

	mu    sync.Mutex
	calls []string
}

// Run returns the scripted output for the command line "name arg1 arg2".
func (r *Runner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	line := strings.Join(append([]string{name}, args...), " ")
	r.mu.Lock()
	r.calls = append(r.calls, line)
	r.mu.Unlock()
	return []byte(r.Outputs[line]), r.Errors[line]
}

// Available reports whether the tool was listed in Tools.
func (r *Runner) Available(name string) bool {
	return r.Tools[name]
}

// Calls returns the command lines run so far.
func (r *Runner) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}
