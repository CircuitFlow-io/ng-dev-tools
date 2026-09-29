package rules

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/nasserghiasi/ng-dev-tools/internal/cleanup"
	"github.com/nasserghiasi/ng-dev-tools/internal/fsx"
)

const dev = cleanup.CategoryDeveloper

func developerRules() []cleanup.Rule {
	return []cleanup.Rule{
		safe(dev, "Xcode DerivedData", "Library/Developer/Xcode/DerivedData"),
		safe(dev, "Xcode caches", "Library/Caches/com.apple.dt.Xcode", "Library/Developer/Xcode/UserData/Previews", "Library/Developer/Xcode/Products"),
		safe(dev, "Simulator caches", "Library/Developer/CoreSimulator/Caches"),
		separate(staleOnly(review(dev, "Xcode archive", "Library/Developer/Xcode/Archives/*/*.xcarchive"))),
		separate(staleOnly(safe(dev, "Device support",
			"Library/Developer/Xcode/iOS DeviceSupport/*",
			"Library/Developer/Xcode/watchOS DeviceSupport/*",
			"Library/Developer/Xcode/tvOS DeviceSupport/*",
			"Library/Developer/Xcode/visionOS DeviceSupport/*",
		))),

		safe(dev, "npm cache", ".npm/_cacache", ".npm/_npx", ".npm/_logs"),
		safe(dev, "Yarn cache", "Library/Caches/Yarn", ".yarn/berry/cache", ".cache/yarn"),
		safe(dev, "pnpm store", "Library/pnpm/store", ".local/share/pnpm/store", ".pnpm-store"),
		safe(dev, "Bun cache", ".bun/install/cache"),
		safe(dev, "node-gyp headers", "Library/Caches/node-gyp", ".node-gyp", ".cache/node-gyp"),
		safe(dev, "TypeScript cache", "Library/Caches/typescript"),
		safe(dev, "Deno cache", "Library/Caches/deno"),
		safe(dev, "Playwright browsers", "Library/Caches/ms-playwright"),
		safe(dev, "Puppeteer browsers", ".cache/puppeteer"),

		safe(dev, "Go build cache", "Library/Caches/go-build"),
		safe(dev, "golangci-lint cache", "Library/Caches/golangci-lint"),
		safe(dev, "Gradle caches", ".gradle/caches", ".gradle/wrapper/dists", ".gradle/daemon", ".gradle/native"),
		safe(dev, "Android cache", ".android/cache"),
		separate(staleOnly(review(dev, "Android emulator", ".android/avd/*.avd"))),
		safe(dev, "Maven repository", ".m2/repository"),
		safe(dev, "CocoaPods cache", "Library/Caches/CocoaPods", ".cocoapods/repos/trunk"),
		safe(dev, "Carthage cache", "Library/Caches/org.carthage.CarthageKit"),
		safe(dev, "Swift Package Manager cache", "Library/Caches/org.swift.swiftpm"),
		safe(dev, "pip cache", "Library/Caches/pip", ".cache/pip"),
		safe(dev, "uv cache", ".cache/uv", "Library/Caches/uv"),
		safe(dev, "Poetry cache", "Library/Caches/pypoetry"),
		safe(dev, "Cargo registry", ".cargo/registry/cache", ".cargo/registry/src", ".cargo/git/checkouts"),
		safe(dev, "Bundler cache", ".bundle/cache"),
		safe(dev, "Composer cache", ".composer/cache", ".cache/composer"),
		review(dev, "Dart pub cache", ".pub-cache"),
		safe(dev, "Terraform plugin cache", ".terraform.d/plugin-cache"),
		safe(dev, "JetBrains caches", "Library/Caches/JetBrains"),
		jetBrainsOldVersions(),

		goModCacheRule{},
		homebrewRule{},
		simulatorsRule{},
		simRuntimesRule{},
		dockerRule{},
	}
}

// jetBrainsOldVersions matches versioned IDE folders such as "GoLand2024.3" that have gone stale.
func jetBrainsOldVersions() PathRule {
	return separate(staleOnly(review(dev, "JetBrains old IDE data", "Library/Application Support/JetBrains/*20[0-9][0-9].[0-9]*")))
}

// commandRule describes an item that a tool should remove itself, such as `go clean -modcache`
// for a cache whose files are read-only.
type commandRule struct {
	title   string
	tool    string
	pathCmd []string
	remove  []string
}

func (r commandRule) scan(ctx context.Context, env cleanup.Env, fallback string) []cleanup.Item {
	if !env.Runner.Available(r.tool) {
		return nil
	}
	path := fallback
	if out, err := env.Runner.Run(ctx, r.tool, r.pathCmd...); err == nil && strings.TrimSpace(string(out)) != "" {
		path = filepath.Clean(strings.TrimSpace(string(out)))
	}
	if !fsx.Exists(path) {
		return nil
	}
	item := cleanup.PathItem(dev, cleanup.SafetySafe, r.title, path)
	item.RemoveCommand = append([]string{r.tool}, r.remove...)
	return []cleanup.Item{item}
}

type goModCacheRule struct{}

func (goModCacheRule) Name() string               { return "Go module cache" }
func (goModCacheRule) Category() cleanup.Category { return dev }

func (goModCacheRule) Scan(ctx context.Context, env cleanup.Env) ([]cleanup.Item, error) {
	rule := commandRule{title: "Go module cache", tool: "go", pathCmd: []string{"env", "GOMODCACHE"}, remove: []string{"clean", "-modcache"}}
	return rule.scan(ctx, env, env.InHome("go", "pkg", "mod")), nil
}

type homebrewRule struct{}

func (homebrewRule) Name() string               { return "Homebrew cache" }
func (homebrewRule) Category() cleanup.Category { return dev }

func (homebrewRule) Scan(ctx context.Context, env cleanup.Env) ([]cleanup.Item, error) {
	rule := commandRule{title: "Homebrew cache & old versions", tool: "brew", pathCmd: []string{"--cache"}, remove: []string{"cleanup", "--prune=all", "-s"}}
	return rule.scan(ctx, env, env.InHome("Library", "Caches", "Homebrew")), nil
}
