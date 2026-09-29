package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/nasserghiasi/ng-dev-tools/internal/fsx"
	"github.com/nasserghiasi/ng-dev-tools/internal/macos"
	"github.com/nasserghiasi/ng-dev-tools/internal/ui"
)

const (
	gigabyte          = 1 << 30
	cleanFix          = "ngt clean"
	oldRuntimesLimit  = 20 * gigabyte
	metroCacheDirName = "metro-cache"
)

// cache is a build cache that grows without bound, and the size worth flagging.
type cache struct {
	name  string
	path  func(ctx context.Context, env Env) string
	limit int64
}

func inHome(parts ...string) func(context.Context, Env) string {
	return func(_ context.Context, env Env) string { return env.homePath(parts...) }
}

var caches = []cache{
	{"Xcode DerivedData", inHome("Library", "Developer", "Xcode", "DerivedData"), 20 * gigabyte},
	{"iOS DeviceSupport", inHome("Library", "Developer", "Xcode", "iOS DeviceSupport"), 10 * gigabyte},
	{"Simulator devices", inHome("Library", "Developer", "CoreSimulator", "Devices"), 30 * gigabyte},
	{"Gradle caches", inHome(".gradle", "caches"), 10 * gigabyte},
	{"Metro cache", metroCachePath, 2 * gigabyte},
	{"pnpm store", inHome("Library", "pnpm", "store"), 20 * gigabyte},
	{"npm cache", inHome(".npm", "_cacache"), 5 * gigabyte},
	{"Go build cache", inHome("Library", "Caches", "go-build"), 10 * gigabyte},
	{"Go module cache", goModCachePath, 10 * gigabyte},
	{"Docker disk image", inHome("Library", "Containers", "com.docker.docker", "Data", "vms", "0", "data", "Docker.raw"), 40 * gigabyte},
}

func cacheChecks() []Check {
	checks := make([]Check, 0, len(caches)+1)
	for _, c := range caches {
		checks = append(checks, Check{Name: c.name, Group: GroupCaches, Run: c.check})
	}
	return append(checks, Check{Name: "Old simulator runtimes", Group: GroupCaches, Run: checkOldRuntimes})
}

func metroCachePath(_ context.Context, env Env) string {
	tmp := env.Getenv("TMPDIR")
	if tmp == "" {
		tmp = os.TempDir()
	}
	return filepath.Join(tmp, metroCacheDirName)
}

func goModCachePath(ctx context.Context, env Env) string {
	if env.installed("go") {
		if dir, err := env.output(ctx, "go", "env", "GOMODCACHE"); err == nil && dir != "" {
			return dir
		}
	}
	return env.homePath("go", "pkg", "mod")
}

func (c cache) check(ctx context.Context, env Env) Result {
	path := c.path(ctx, env)
	if !fsx.Exists(path) {
		return pass("none")
	}
	size := fsx.DiskUsage(ctx, path)
	if size > c.limit {
		return warn(fmt.Sprintf("%s, over %s", ui.Bytes(size), ui.Bytes(c.limit)), cleanFix)
	}
	return pass(ui.Bytes(size))
}

func checkOldRuntimes(ctx context.Context, env Env) Result {
	sdk, err := env.simulatorSDK(ctx)
	if err != nil {
		return skip("no iOS Simulator SDK")
	}
	runtimes, err := macos.SimRuntimes(ctx, env.Runner)
	if err != nil {
		return skip("could not list simulator runtimes")
	}
	old, total := olderRuntimes(runtimes, sdk)
	if len(old) == 0 {
		return pass("none older than iOS " + fmt.Sprintf("%d.%d", sdk.Major, sdk.Minor))
	}
	summary := fmt.Sprintf("%s, %s", ui.Count(len(old), "runtime"), ui.Bytes(total))
	if total > oldRuntimesLimit {
		return warn(summary, cleanFix).with(old...)
	}
	return pass(summary).with(old...)
}

func olderRuntimes(runtimes []macos.SimRuntime, sdk Version) (names []string, total int64) {
	runtimes = slices.Clone(runtimes)
	slices.SortFunc(runtimes, func(a, b macos.SimRuntime) int {
		return runtimeVersion(b).Compare(runtimeVersion(a))
	})
	for _, rt := range runtimes {
		if !runtimeVersion(rt).Less(sdk) {
			continue
		}
		names = append(names, fmt.Sprintf("iOS %s (%s)", rt.Version, ui.Bytes(rt.SizeBytes)))
		total += rt.SizeBytes
	}
	return names, total
}

func runtimeVersion(rt macos.SimRuntime) Version {
	v, _ := ParseVersion(rt.Version)
	return v
}
