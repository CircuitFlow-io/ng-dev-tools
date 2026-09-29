package scripts

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	pathVariable   = "PATH="
	executableBits = 0o111
)

// Invocation is a script run ready to start: the command, its folder and its environment.
type Invocation struct {
	Dir  string
	Argv []string
	Env  []string
	// Display is the equivalent shell command, such as "cd apps/api && pnpm run dev".
	Display string
}

// Plan builds the command that runs t with the workspace's package manager in t's package folder,
// putting node's bin folder first on PATH when that version is installed.
func Plan(w Workspace, t Target, node Node, env []string) Invocation {
	if node.Installed() {
		env = prependPath(env, node.Bin)
	}
	return Invocation{Dir: t.Package.Dir, Argv: command(w, t), Env: env, Display: DisplayCommand(w, t)}
}

func command(w Workspace, t Target) []string {
	return []string{string(w.Manager), "run", t.Script.Name}
}

// DisplayCommand is the shell command equivalent to running t from the project root.
func DisplayCommand(w Workspace, t Target) string {
	display := strings.Join(command(w, t), " ")
	if t.Package.IsRoot() {
		return display
	}
	return "cd " + t.Package.RelDir + " && " + display
}

func prependPath(env []string, dir string) []string {
	out := make([]string, 0, len(env)+1)
	found := false
	for _, kv := range env {
		if value, ok := strings.CutPrefix(kv, pathVariable); ok {
			kv, found = pathVariable+dir+string(os.PathListSeparator)+value, true
		}
		out = append(out, kv)
	}
	if !found {
		out = append(out, pathVariable+dir)
	}
	return out
}

var errNotFound = errors.New("not found on PATH")

// Exec replaces ngt with the script's command, so the script owns the terminal, gets Ctrl+C
// directly and its exit code becomes ngt's. It only returns on failure.
func Exec(inv Invocation) error {
	program, err := lookPath(inv.Argv[0], inv.Env)
	if err != nil {
		return err
	}
	if err := os.Chdir(inv.Dir); err != nil {
		return err
	}
	return syscall.Exec(program, inv.Argv, inv.Env)
}

// lookPath searches the PATH of env, which may differ from ngt's own.
func lookPath(name string, env []string) (string, error) {
	for _, kv := range env {
		value, ok := strings.CutPrefix(kv, pathVariable)
		if !ok {
			continue
		}
		for _, dir := range filepath.SplitList(value) {
			candidate := filepath.Join(dir, name)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&executableBits != 0 {
				return candidate, nil
			}
		}
	}
	return "", &os.PathError{Op: "exec", Path: name, Err: errNotFound}
}
