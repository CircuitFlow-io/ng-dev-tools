package cli

import (
	"github.com/CircuitFlow-io/ng-dev-tools/internal/scripts"
)

type scriptsJSON struct {
	Root      string       `json:"root"`
	Manager   string       `json:"manager"`
	RunsHooks bool         `json:"runsHooks"`
	Scripts   []scriptJSON `json:"scripts"`
}

type scriptJSON struct {
	// Package is the folder relative to the root, "." for the root itself.
	Package     string `json:"package"`
	PackageName string `json:"packageName,omitempty"`
	Dir         string `json:"dir"`
	Script      string `json:"script"`
	Command     string `json:"command"`
	Pre         string `json:"pre,omitempty"`
	Post        string `json:"post,omitempty"`
}

func toScriptsJSON(ws scripts.Workspace, targets []scripts.Target) scriptsJSON {
	views := make([]scriptJSON, 0, len(targets))
	for _, t := range targets {
		pkg := t.Package.RelDir
		if t.Package.IsRoot() {
			pkg = rootPackageLabel
		}
		views = append(views, scriptJSON{
			Package:     pkg,
			PackageName: t.Package.Name,
			Dir:         t.Package.Dir,
			Script:      t.Script.Name,
			Command:     t.Script.Command,
			Pre:         t.Script.Pre,
			Post:        t.Script.Post,
		})
	}
	return scriptsJSON{Root: ws.Root, Manager: string(ws.Manager), RunsHooks: ws.RunsHooks, Scripts: views}
}
