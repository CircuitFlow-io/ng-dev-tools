package pulls

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
)

const (
	gitConfigFile = "config"
	urlKey        = "url"
)

// githubRemote matches the owner/name of a GitHub remote URL in its ssh, scp-like and https forms.
var githubRemote = regexp.MustCompile(`^(?:https://|ssh://git@|git@)github\.com[:/]([^/]+/[^/]+?)(?:\.git)?/?$`)

// Clones maps each GitHub repository ("owner/name", lower case) to the project in root that is a
// clone of it, judged by its remotes. The first project, by name, wins when there are several.
func Clones(root string) map[string]string {
	dirs, err := projects.Dirs(root)
	if err != nil {
		return nil
	}
	clones := map[string]string{}
	for _, dir := range dirs {
		gitDir := projects.GitDir(dir)
		if gitDir == "" {
			continue
		}
		for _, url := range remoteURLs(filepath.Join(projects.CommonDir(gitDir), gitConfigFile)) {
			if repo, ok := RepoFromURL(url); ok {
				if _, taken := clones[repo]; !taken {
					clones[repo] = dir
				}
			}
		}
	}
	return clones
}

// LocalClone is the project in clones holding repo, if there is one.
func LocalClone(clones map[string]string, repo string) (string, bool) {
	dir, ok := clones[strings.ToLower(repo)]
	return dir, ok
}

// RepoFromURL reads "owner/name", lower case, from a GitHub remote URL.
func RepoFromURL(url string) (string, bool) {
	m := githubRemote.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil {
		return "", false
	}
	return strings.ToLower(m[1]), true
}

// remoteURLs reads every "url = ..." line of a git config file.
func remoteURLs(configPath string) []string {
	f, err := os.Open(configPath)
	if err != nil {
		return nil
	}
	defer f.Close()
	var urls []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if ok && strings.TrimSpace(key) == urlKey {
			urls = append(urls, strings.TrimSpace(value))
		}
	}
	return urls
}
