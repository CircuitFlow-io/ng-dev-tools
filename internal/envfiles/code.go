package envfiles

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// maxSourceBytes skips bundles and generated files, which are large and not the project's own code.
const maxSourceBytes = 512 << 10

const namePattern = `([A-Za-z_][A-Za-z0-9_]*)`

// envReads matches the usual ways code reads a variable: process.env.X, process.env["X"],
// import.meta.env.X, Bun.env.X, Deno.env.get("X"), os.Getenv("X"), os.getenv("X"),
// os.environ["X"], os.environ.get("X"), ENV["X"] and ENV.fetch("X").
var envReads = regexp.MustCompile(
	`(?:\bprocess\.env|\bimport\.meta\.env|\bBun\.env)\.` + namePattern +
		`|\bprocess\.env\[\s*["'` + "`" + `]` + namePattern + `["'` + "`" + `]\s*\]` +
		`|(?:\bos\.Getenv|\bos\.LookupEnv|\bos\.getenv|\bos\.environ\.get|\bDeno\.env\.get|\bENV\.fetch)\(\s*["']` + namePattern + `["']` +
		`|(?:\bos\.environ|\bENV)\[\s*["']` + namePattern + `["']\s*\]`)

var (
	sourceExtensions = map[string]bool{
		".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
		".ts": true, ".tsx": true, ".mts": true, ".cts": true,
		".vue": true, ".svelte": true, ".astro": true,
		".py": true, ".go": true, ".rb": true,
	}
	// sourceNames are fastlane's Ruby files, which have no extension.
	sourceNames = map[string]bool{"Fastfile": true, "Appfile": true, "Matchfile": true, "Pluginfile": true}
)

// runtimeKeys are set by Node, frameworks, CI or the shell rather than by an env file.
var (
	runtimeKeys = map[string]bool{
		"NODE_ENV": true, "CI": true, "TZ": true, "HOME": true, "PATH": true, "PWD": true, "USER": true,
		"SHELL": true, "PORT": true, "HOSTNAME": true, "NEXT_RUNTIME": true, "NEXT_PHASE": true,
		"EXPO_OS": true, "INIT_CWD": true, "MODE": true, "DEV": true, "PROD": true, "SSR": true, "BASE_URL": true,
	}
	runtimePrefixes = []string{"GITHUB_", "RUNNER_", "VERCEL_", "NETLIFY_", "EAS_BUILD", "npm_", "JEST_", "VITEST"}
)

func isSource(name string) bool {
	return sourceNames[name] || (sourceExtensions[filepath.Ext(name)] && !strings.HasSuffix(name, ".min.js"))
}

func isRuntimeKey(key string) bool {
	if runtimeKeys[key] {
		return true
	}
	for _, prefix := range runtimePrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// attachCode puts on each folder's first set the keys its code reads that no example or local
// file there names. Code belongs to the closest folder above it that has an example.
func (p *project) attachCode(projectDir string, sources []string) {
	primary := map[string]int{}
	for i, s := range p.sets {
		if _, seen := primary[s.Dir]; !seen && s.Example != "" {
			primary[s.Dir] = i
		}
	}
	reported := map[string]map[string]bool{}
	for _, path := range sources {
		folder, ok := closestFolder(filepath.Dir(path), projectDir, primary)
		if !ok {
			continue
		}
		s := &p.sets[primary[folder]]
		for _, ref := range readsIn(path) {
			if p.known[folder][ref.Key] || isRuntimeKey(ref.Key) || reported[folder][ref.Key] {
				continue
			}
			if reported[folder] == nil {
				reported[folder] = map[string]bool{}
			}
			reported[folder][ref.Key] = true
			ref.File, _ = filepath.Rel(folder, path)
			s.InCode = append(s.InCode, ref)
		}
	}
}

// closestFolder walks up from dir to projectDir and returns the first folder that has an example.
func closestFolder(dir, projectDir string, folders map[string]int) (string, bool) {
	for {
		if _, ok := folders[dir]; ok {
			return dir, true
		}
		if dir == projectDir || !strings.HasPrefix(dir, projectDir) {
			return "", false
		}
		dir = filepath.Dir(dir)
	}
}

// readsIn finds the variables a source file reads, first read of each first.
func readsIn(path string) []CodeRef {
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxSourceBytes {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var refs []CodeRef
	seen := map[string]bool{}
	for _, match := range envReads.FindAllSubmatchIndex(content, -1) {
		key := matchedName(content, match)
		if seen[key] {
			continue
		}
		seen[key] = true
		refs = append(refs, CodeRef{Key: key, Line: 1 + bytes.Count(content[:match[0]], []byte("\n"))})
	}
	return refs
}

// matchedName is whichever of the pattern's alternatives matched.
func matchedName(content []byte, match []int) string {
	for group := 1; 2*group+1 < len(match); group++ {
		if start := match[2*group]; start >= 0 {
			return string(content[start:match[2*group+1]])
		}
	}
	return ""
}
