package doctor

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/fsx"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	recommendedJDK       = 17
	minAndroidAPI        = 35
	minBuildToolsMajor   = 35
	defaultAndroidSDK    = "Library/Android/sdk"
	androidStudioApp     = "/Applications/Android Studio.app"
	sdkToolsMenu         = "Android Studio > Settings > Languages & Frameworks > Android SDK > "
	jdkInstallFix        = "brew install --cask zulu@17"
	exportJavaHomeFix    = `add to ~/.zshrc: export JAVA_HOME="$(/usr/libexec/java_home -v 17)"`
	exportAndroidHomeFix = `add to ~/.zshrc: export ANDROID_HOME="$HOME/Library/Android/sdk"`
)

func androidChecks() []Check {
	return []Check{
		{Name: "JDK", Group: GroupAndroid, Run: checkJDK},
		{Name: "JAVA_HOME", Group: GroupAndroid, Run: checkJavaHome},
		{Name: "ANDROID_HOME", Group: GroupAndroid, Run: checkAndroidHome},
		{Name: "Android Studio", Group: GroupAndroid, Run: checkAndroidStudio},
		{Name: "Platform tools (adb)", Group: GroupAndroid, Run: sdkBinary{
			name: "adb", dir: "platform-tools", component: "SDK Tools > Android SDK Platform-Tools",
		}.check},
		{Name: "Emulator", Group: GroupAndroid, Run: sdkBinary{
			name: "emulator", dir: "emulator", component: "SDK Tools > Android Emulator",
		}.check},
		{Name: "Command-line tools", Group: GroupAndroid, Run: checkCmdlineTools},
		{Name: "Build tools", Group: GroupAndroid, Run: checkBuildTools},
		{Name: "SDK platform", Group: GroupAndroid, Run: checkSDKPlatform},
		{Name: "NDK", Group: GroupAndroid, Run: checkNDK},
		{Name: "Virtual device", Group: GroupAndroid, Run: checkAVD},
	}
}

func checkJDK(ctx context.Context, env Env) Result {
	v, err := env.version(ctx, "java", "--version")
	if err != nil {
		return missing(err, jdkInstallFix)
	}
	if v.Major != recommendedJDK {
		return warn(fmt.Sprintf("JDK %d, React Native expects %d", v.Major, recommendedJDK), jdkInstallFix+", then point JAVA_HOME at it")
	}
	return pass(v.String())
}

func checkJavaHome(_ context.Context, env Env) Result {
	home := env.Getenv("JAVA_HOME")
	if home == "" {
		return fail("not set", exportJavaHomeFix)
	}
	if !fsx.Exists(home) {
		return fail("points to a missing directory: "+home, exportJavaHomeFix)
	}
	return pass(ui.TildePath(home, env.Home))
}

func checkAndroidHome(_ context.Context, env Env) Result {
	sdk := env.exportedAndroidSDK()
	fallback := env.homePath(defaultAndroidSDK)
	switch {
	case sdk == "" && fsx.Exists(fallback):
		return warn("not exported; the SDK is at "+ui.TildePath(fallback, env.Home), exportAndroidHomeFix)
	case sdk == "":
		return fail("no Android SDK found", "install Android Studio and finish its setup wizard")
	case !fsx.Exists(sdk):
		return fail("points to a missing directory: "+sdk, exportAndroidHomeFix)
	default:
		return pass(ui.TildePath(sdk, env.Home))
	}
}

func (e Env) exportedAndroidSDK() string {
	if sdk := e.Getenv("ANDROID_HOME"); sdk != "" {
		return sdk
	}
	return e.Getenv("ANDROID_SDK_ROOT")
}

// androidSDK is the SDK in use: the exported one, or Android Studio's default location.
func (e Env) androidSDK() (string, bool) {
	for _, sdk := range []string{e.exportedAndroidSDK(), e.homePath(defaultAndroidSDK)} {
		if sdk != "" && fsx.Exists(sdk) {
			return sdk, true
		}
	}
	return "", false
}

var noSDK = skip("no Android SDK")

func checkAndroidStudio(_ context.Context, env Env) Result {
	for _, app := range []string{env.systemPath(androidStudioApp), env.homePath(androidStudioApp)} {
		if fsx.Exists(app) {
			return pass(ui.TildePath(app, env.Home))
		}
	}
	return warn("not installed", "brew install --cask android-studio")
}

// sdkBinary is a tool shipped inside the Android SDK that should also be on PATH.
type sdkBinary struct {
	name      string
	dir       string
	component string
}

func (b sdkBinary) check(_ context.Context, env Env) Result {
	sdk, ok := env.androidSDK()
	if !ok {
		return noSDK
	}
	binDir := filepath.Join(sdk, b.dir)
	if !fsx.Exists(filepath.Join(binDir, b.name)) {
		return fail("not installed", sdkToolsMenu+b.component)
	}
	if !env.installed(b.name) {
		return warn("installed but not on PATH", fmt.Sprintf(`add to ~/.zshrc: export PATH="$ANDROID_HOME/%s:$PATH"`, b.dir))
	}
	return pass(ui.TildePath(binDir, env.Home))
}

func checkCmdlineTools(_ context.Context, env Env) Result {
	sdk, ok := env.androidSDK()
	if !ok {
		return noSDK
	}
	if !fsx.Exists(filepath.Join(sdk, "cmdline-tools", "latest", "bin", "sdkmanager")) {
		return fail("sdkmanager and avdmanager are missing", sdkToolsMenu+"SDK Tools > Android SDK Command-line Tools (latest)")
	}
	return pass("installed")
}

func checkBuildTools(_ context.Context, env Env) Result {
	sdk, ok := env.androidSDK()
	if !ok {
		return noSDK
	}
	newest, found := newestVersion(subdirs(filepath.Join(sdk, "build-tools")), "")
	fix := sdkToolsMenu + fmt.Sprintf("SDK Tools > Android SDK Build-Tools %d", minBuildToolsMajor)
	if !found {
		return fail("none installed", fix)
	}
	if newest.Major < minBuildToolsMajor {
		return fail(fmt.Sprintf("newest is %s, need %d or newer", newest, minBuildToolsMajor), fix)
	}
	return pass(newest.String())
}

func checkSDKPlatform(_ context.Context, env Env) Result {
	sdk, ok := env.androidSDK()
	if !ok {
		return noSDK
	}
	newest, found := newestVersion(subdirs(filepath.Join(sdk, "platforms")), "android-")
	fix := sdkToolsMenu + fmt.Sprintf("SDK Platforms > Android API %d", minAndroidAPI)
	if !found {
		return fail("no platform installed", fix)
	}
	if newest.Major < minAndroidAPI {
		return fail(fmt.Sprintf("newest is API %d, need %d or newer", newest.Major, minAndroidAPI), fix)
	}
	return pass(fmt.Sprintf("API %d", newest.Major))
}

func checkNDK(_ context.Context, env Env) Result {
	sdk, ok := env.androidSDK()
	if !ok {
		return noSDK
	}
	newest, found := newestVersion(subdirs(filepath.Join(sdk, "ndk")), "")
	if !found {
		return warn("not installed; the first native build downloads it", sdkToolsMenu+"SDK Tools > NDK (Side by side)")
	}
	return pass(newest.String())
}

// newestVersion finds the highest version among directory names that start with prefix.
func newestVersion(names []string, prefix string) (Version, bool) {
	var versions []Version
	for _, name := range names {
		rest, ok := strings.CutPrefix(name, prefix)
		if v, parsed := ParseVersion(rest); ok && parsed {
			versions = append(versions, v)
		}
	}
	if len(versions) == 0 {
		return Version{}, false
	}
	return slices.MaxFunc(versions, Version.Compare), true
}

func checkAVD(_ context.Context, env Env) Result {
	matches, _ := filepath.Glob(env.homePath(".android", "avd", "*.avd"))
	if len(matches) == 0 {
		return warn("no virtual device", "Android Studio > Device Manager > Create Virtual Device")
	}
	names := make([]string, len(matches))
	for i, match := range matches {
		names[i] = strings.TrimSuffix(filepath.Base(match), ".avd")
	}
	return pass(strings.Join(names, ", "))
}
