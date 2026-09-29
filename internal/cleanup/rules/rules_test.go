package rules

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/nasserghiasi/ng-dev-tools/internal/cleanup"
	"github.com/nasserghiasi/ng-dev-tools/internal/macos"
	"github.com/nasserghiasi/ng-dev-tools/internal/macos/macostest"
)

const staleWindow = 90 * 24 * time.Hour

var longAgo = time.Now().Add(-2 * staleWindow)

func testEnv(t *testing.T) cleanup.Env {
	t.Helper()
	return cleanup.Env{
		Home:             t.TempDir(),
		Now:              time.Now(),
		StaleAfter:       staleWindow,
		LargeFileMinSize: 1024,
		Runner:           &macostest.Runner{},
	}
}

func touch(t *testing.T, path string, size int, modified time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
}

func age(t *testing.T, path string, modified time.Time) {
	t.Helper()
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
}

func titles(items []cleanup.Item) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Title)
	}
	slices.Sort(out)
	return out
}

func TestPathRuleGroupsSeparatesAndExcludes(t *testing.T) {
	env := testEnv(t)
	touch(t, env.InHome("Library/Caches/com.example/a"), 1, time.Now())
	touch(t, env.InHome("Library/Caches/com.apple.x/a"), 1, time.Now())
	touch(t, env.InHome("Library/Caches/Other/a"), 1, time.Now())

	grouped, _ := safe(caches, "All", "Library/Caches/*").Scan(context.Background(), env)
	perApp, _ := excluding(separate(safe(caches, "App", "Library/Caches/*")), "com.apple.*").Scan(context.Background(), env)

	if len(grouped) != 1 || len(grouped[0].Paths) != 3 {
		t.Errorf("grouped = %+v, want one item with 3 paths", grouped)
	}
	if got, want := titles(perApp), []string{"App: Other", "App: com.example"}; !slices.Equal(got, want) {
		t.Errorf("separate = %v, want %v", got, want)
	}
}

func TestPathRuleStaleOnly(t *testing.T) {
	env := testEnv(t)
	touch(t, env.InHome("DeviceSupport/17.0/f"), 1, longAgo)
	age(t, env.InHome("DeviceSupport/17.0"), longAgo)
	touch(t, env.InHome("DeviceSupport/18.0/f"), 1, time.Now())

	items, _ := separate(staleOnly(safe(dev, "DS", "DeviceSupport/*"))).Scan(context.Background(), env)

	if got := titles(items); !slices.Equal(got, []string{"DS: 17.0"}) {
		t.Errorf("items = %v, want only the stale version", got)
	}
}

func TestProjectsRuleFindsArtifactsOfStaleProjectsOnly(t *testing.T) {
	env := testEnv(t)
	root := env.InHome("projects")
	env.ProjectDirs = []string{root}

	stale := filepath.Join(root, "old-app")
	touch(t, filepath.Join(stale, "package.json"), 1, longAgo)
	touch(t, filepath.Join(stale, "node_modules", "left-pad", "index.js"), 1, time.Now())
	age(t, stale, longAgo)

	fresh := filepath.Join(root, "new-app")
	touch(t, filepath.Join(fresh, "package.json"), 1, time.Now())
	touch(t, filepath.Join(fresh, "node_modules", "x.js"), 1, time.Now())

	unmarked := filepath.Join(root, "notes")
	touch(t, filepath.Join(unmarked, "target", "file"), 1, longAgo)
	age(t, unmarked, longAgo)

	items, err := projectsRule{}.Scan(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if got := titles(items); !slices.Equal(got, []string{"node_modules in old-app"}) {
		t.Errorf("items = %v", got)
	}
}

func TestLargeFilesRule(t *testing.T) {
	env := testEnv(t)
	touch(t, env.InHome("Downloads/big.mov"), 4096, longAgo)
	touch(t, env.InHome("Downloads/Installer.dmg"), 2<<20, longAgo)
	touch(t, env.InHome("Downloads/recent.mov"), 4096, time.Now())
	touch(t, env.InHome("Downloads/small.txt"), 10, longAgo)
	touch(t, env.InHome("Downloads/Tool.app/Contents/big"), 4096, longAgo)

	items, err := largeFilesRule{}.Scan(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := titles(items), []string{"Installer.dmg", "big.mov"}; !slices.Equal(got, want) {
		t.Errorf("items = %v, want %v", got, want)
	}
}

func TestSimulatorItem(t *testing.T) {
	env := testEnv(t)
	tests := []struct {
		name      string
		available bool
		lastUsed  time.Time
		wantTitle string
	}{
		{"unavailable", false, time.Now(), "Unavailable simulator: iPhone (iOS 18.6)"},
		{"stale", true, longAgo, "Unused simulator: iPhone (iOS 18.6)"},
		{"recent", true, time.Now(), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sim := macosSimulator(tt.available, tt.lastUsed)
			item, ok := simulatorItem(env, sim)
			if ok != (tt.wantTitle != "") || item.Title != tt.wantTitle {
				t.Errorf("got (%q, %v), want %q", item.Title, ok, tt.wantTitle)
			}
			if ok && !slices.Equal(item.RemoveCommand, []string{"xcrun", "simctl", "delete", "U1"}) {
				t.Errorf("RemoveCommand = %v", item.RemoveCommand)
			}
		})
	}
}

func macosSimulator(available bool, lastUsed time.Time) macos.Simulator {
	return macos.Simulator{
		UDID:        "U1",
		Name:        "iPhone",
		DataPath:    "/nonexistent/U1/data",
		IsAvailable: available,
		LastUsedAt:  lastUsed,
		RuntimeID:   "com.apple.CoreSimulator.SimRuntime.iOS-18-6",
	}
}

func TestParseDockerDF(t *testing.T) {
	out := []byte(`{"Type":"Images","Reclaimable":"3.4GB (40%)"}
{"Type":"Containers","Reclaimable":"0B (0%)"}
{"Type":"Local Volumes","Reclaimable":"9GB (100%)"}
{"Type":"Build Cache","Reclaimable":"1.2GB"}`)

	items := parseDockerDF(out)

	if got, want := titles(items), []string{"Docker build cache", "Docker unused images"}; !slices.Equal(got, want) {
		t.Errorf("items = %v, want %v (volumes and empty rows skipped)", got, want)
	}
}

func TestRuntimeName(t *testing.T) {
	if got := runtimeName("com.apple.CoreSimulator.SimRuntime.iOS-18-6"); got != "iOS 18.6" {
		t.Errorf("runtimeName = %q", got)
	}
}

func TestBundleIDSetIsConservative(t *testing.T) {
	installed := make(bundleIDSet)
	installed.add("com.google.Chrome")

	for id, want := range map[string]bool{
		"com.google.Chrome":           true,
		"com.google.Keystone.Agent":   true,
		"COM.GOOGLE.chrome":           true,
		"com.tinyapp.TablePlus":       false,
		"com.googleusercontent.thing": false,
	} {
		if got := installed.covers(id); got != want {
			t.Errorf("covers(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestLeftoversRuleSkipsWhenSpotlightLooksEmpty(t *testing.T) {
	env := testEnv(t)
	env.Runner = &macostest.Runner{Outputs: map[string]string{
		"mdfind -0 kMDItemContentType == 'com.apple.application-bundle'": "/Applications/Only.app\x00",
	}}

	if _, err := (leftoversRule{}).Scan(context.Background(), env); err == nil {
		t.Error("expected an error when Spotlight reports almost no apps")
	}
}

func TestDefaultRulesHaveUniqueNames(t *testing.T) {
	seen := make(map[string]bool)
	for _, rule := range Default() {
		if seen[rule.Name()] {
			t.Errorf("duplicate rule name %q", rule.Name())
		}
		seen[rule.Name()] = true
	}
}
