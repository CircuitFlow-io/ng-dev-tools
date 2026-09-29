package doctor

import (
	"context"
	"fmt"
	"strings"

	"github.com/nasserghiasi/ng-dev-tools/internal/macos"
	"github.com/nasserghiasi/ng-dev-tools/internal/ui"
)

const (
	systemRuby      = "/usr/bin/ruby"
	xcodeDevDirPart = ".app/Contents/Developer"
	selectXcodeFix  = "sudo xcode-select -s /Applications/Xcode.app/Contents/Developer"
)

var (
	minXcode     = Version{Major: 16}
	minCocoaPods = Version{Major: 1, Minor: 15}
	minRuby      = Version{Major: 3}
)

func iosChecks() []Check {
	return []Check{
		{Name: "Xcode", Group: GroupIOS, Run: toolSpec{
			command: "xcodebuild", args: []string{"-version"}, minimum: minXcode,
			fix: "Install Xcode from the App Store, then run: " + selectXcodeFix,
		}.check},
		{Name: "Xcode developer dir", Group: GroupIOS, Run: checkXcodeSelect},
		{Name: "Xcode first launch", Group: GroupIOS, Run: checkXcodeFirstLaunch},
		{Name: "iOS Simulator runtime", Group: GroupIOS, Run: checkSimulatorRuntime},
		{Name: "CocoaPods", Group: GroupIOS, Run: toolSpec{
			command: "pod", args: []string{"--version"}, minimum: minCocoaPods, fix: "brew install cocoapods",
		}.check},
		{Name: "Ruby", Group: GroupIOS, Run: checkRuby},
		{Name: "Watchman", Group: GroupIOS, Run: toolSpec{
			command: "watchman", args: []string{"--version"}, fix: "brew install watchman",
		}.check},
		{Name: "EAS CLI", Group: GroupIOS, Run: toolSpec{
			command: "eas", args: []string{"--version"}, fix: "npm install -g eas-cli", optional: true,
		}.check},
	}
}

func checkXcodeSelect(ctx context.Context, env Env) Result {
	path, err := env.output(ctx, "xcode-select", "-p")
	if err != nil {
		return fail("no developer directory selected", selectXcodeFix).with(errorLine(err))
	}
	if !strings.Contains(path, xcodeDevDirPart) {
		return fail("points to "+path+", not Xcode", selectXcodeFix)
	}
	return pass(path)
}

func checkXcodeFirstLaunch(ctx context.Context, env Env) Result {
	if !env.installed("xcodebuild") {
		return skip("Xcode is not installed")
	}
	if _, err := env.output(ctx, "xcodebuild", "-checkFirstLaunchStatus"); err != nil {
		return fail("license or first-launch components are missing", "sudo xcodebuild -license accept && sudo xcodebuild -runFirstLaunch")
	}
	return pass("license accepted, components installed")
}

func checkSimulatorRuntime(ctx context.Context, env Env) Result {
	sdk, err := env.simulatorSDK(ctx)
	if err != nil {
		return skip("no iOS Simulator SDK (is Xcode installed?)")
	}
	runtimes, err := macos.SimRuntimes(ctx, env.Runner)
	if err != nil {
		return fail("could not list simulator runtimes", "").with(errorLine(err))
	}
	for _, rt := range runtimes {
		if v, ok := ParseVersion(rt.Version); ok && v.Major == sdk.Major && v.Minor == sdk.Minor {
			return pass(fmt.Sprintf("iOS %d.%d (%s)", v.Major, v.Minor, ui.Bytes(rt.SizeBytes)))
		}
	}
	return fail(fmt.Sprintf("no runtime for the iOS %d.%d SDK", sdk.Major, sdk.Minor), "xcodebuild -downloadPlatform iOS")
}

func (e Env) simulatorSDK(ctx context.Context) (Version, error) {
	out, err := e.output(ctx, "xcrun", "--sdk", "iphonesimulator", "--show-sdk-version")
	if err != nil {
		return Version{}, err
	}
	return parseRelease(out)
}

func checkRuby(ctx context.Context, env Env) Result {
	path, err := env.LookPath("ruby")
	if err != nil {
		return fail("not installed", "brew install ruby")
	}
	if path == systemRuby {
		return warn("using the macOS system Ruby", `brew install ruby, then add "$(brew --prefix ruby)/bin" to the front of PATH`)
	}
	return toolSpec{command: "ruby", args: []string{"--version"}, minimum: minRuby, fix: "brew upgrade ruby"}.check(ctx, env).
		with(ui.TildePath(path, env.Home))
}
