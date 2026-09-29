package scripts

import "regexp"

// maxReferenceDepth bounds how far References follows scripts that call scripts.
const maxReferenceDepth = 3

// scriptCall matches "npm run x", "pnpm run x", "npm run-script x" and the shorthand "pnpm x".
var scriptCall = regexp.MustCompile(`\b(?:npm|pnpm)\s+(?:run(?:-script)?\s+)?([A-Za-z0-9_][\w:.\-]*)`)

// Reference is a script called by another script, Depth levels down.
type Reference struct {
	Script Script
	Depth  int
}

// References lists the scripts of pkg that script calls, and the ones those call, in the order
// they appear. Each is listed once, so scripts calling each other do not loop.
func References(pkg Package, script Script) []Reference {
	seen := map[string]bool{script.Name: true}
	var refs []Reference
	var follow func(command string, depth int)
	follow = func(command string, depth int) {
		if depth > maxReferenceDepth {
			return
		}
		for _, match := range scriptCall.FindAllStringSubmatch(command, -1) {
			called, ok := pkg.Script(match[1])
			if !ok || seen[called.Name] {
				continue
			}
			seen[called.Name] = true
			refs = append(refs, Reference{Script: called, Depth: depth})
			follow(called.Command, depth+1)
		}
	}
	follow(script.Command, 1)
	return refs
}
