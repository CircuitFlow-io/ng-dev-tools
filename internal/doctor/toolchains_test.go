package doctor

import (
	"errors"
	"path/filepath"
	"testing"
)

const corepackCrash = `pnpm --version: exit status 1: /x/corepack.cjs:21535
  if (key == null || signature == null) throw new Error(...);
                                              ^

Error: Cannot find matching keyid: {"signatures":[]}
    at verifySignature (/x/corepack.cjs:21535:47)`

func TestJudgeNode(t *testing.T) {
	lts := Version{24, 9, 0}
	tests := []struct {
		name    string
		current Version
		err     error
		want    Status
	}{
		{"older major", Version{22, 13, 1}, nil, StatusFail},
		{"newer major", Version{25, 0, 0}, nil, StatusWarn},
		{"older release of the LTS", Version{24, 1, 0}, nil, StatusWarn},
		{"latest LTS", Version{24, 9, 0}, nil, StatusPass},
		{"offline", Version{22, 0, 0}, ErrOffline, StatusPass},
		{"lookup failed", Version{24, 9, 0}, errNoRelease, StatusWarn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertStatus(t, judgeNode(tt.current, lts, tt.err), tt.want)
		})
	}
}

func TestNodeShowsNvmDefault(t *testing.T) {
	m := newMachine(t)
	m.install("node", "node --version", "v22.13.1")
	m.releases.node = Version{24, 9, 0}
	m.file(".nvm/alias/default")

	result := m.run(checkNode)

	assertStatus(t, result, StatusFail)
	assertContains(t, result.Fix, "nvm install --lts")
	if len(result.Details) != 1 {
		t.Errorf("details = %q, want the nvm default", result.Details)
	}
}

func TestPnpm(t *testing.T) {
	tests := []struct {
		name    string
		version string
		err     error
		want    Status
		fix     string
	}{
		{"corepack shim crashes", "", errors.New(corepackCrash), StatusFail, corepackPnpmFix},
		{"too old", "10.22.0", nil, StatusFail, pnpmInstallFix},
		{"behind latest", "11.1.0", nil, StatusWarn, pnpmInstallFix},
		{"latest", "12.6.0", nil, StatusPass, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMachine(t)
			m.install("pnpm", "pnpm --version", tt.version)
			m.failing("pnpm --version", tt.err)
			m.releases.npm["pnpm"] = Version{12, 6, 0}

			result := m.run(checkPnpm)

			assertStatus(t, result, tt.want)
			if result.Fix != tt.fix {
				t.Errorf("fix = %q, want %q", result.Fix, tt.fix)
			}
		})
	}
}

func TestCorepackCrashShowsTheErrorLine(t *testing.T) {
	result := pnpmMissing(errors.New(corepackCrash))

	assertContains(t, result.Details[0], "Error: Cannot find matching keyid")
}

func TestPnpmNotInstalled(t *testing.T) {
	result := newMachine(t).run(checkPnpm)

	assertStatus(t, result, StatusFail)
	assertContains(t, result.Summary, "not installed")
}

func TestXcodeSelectMustPointIntoXcode(t *testing.T) {
	m := newMachine(t)
	m.install("xcode-select", "xcode-select -p", "/Library/Developer/CommandLineTools")

	result := m.run(checkXcodeSelect)

	assertStatus(t, result, StatusFail)
	assertContains(t, result.Fix, "xcode-select -s")
}

func TestSimulatorRuntimeMatchesSDK(t *testing.T) {
	m := newMachine(t)
	m.install("xcrun", "xcrun --sdk iphonesimulator --show-sdk-version", "27.0")
	m.runner.Outputs["xcrun simctl runtime list -j"] = `{"A":{"version":"26.5","sizeBytes":1},"B":{"version":"27.0","sizeBytes":8067000161}}`

	assertStatus(t, m.run(checkSimulatorRuntime), StatusPass)

	m.runner.Outputs["xcrun --sdk iphonesimulator --show-sdk-version"] = "27.1"
	assertStatus(t, m.run(checkSimulatorRuntime), StatusFail)
}

func TestSystemRubyWarns(t *testing.T) {
	m := newMachine(t)
	m.tools["ruby"] = systemRuby

	assertStatus(t, m.run(checkRuby), StatusWarn)
}

func TestMinimumVersion(t *testing.T) {
	m := newMachine(t)
	m.install("pod", "pod --version", "1.14.3")
	spec := toolSpec{command: "pod", args: []string{"--version"}, minimum: minCocoaPods, fix: "brew install cocoapods"}

	assertStatus(t, spec.check(t.Context(), m.env()), StatusFail)
}

func TestAndroidSDKFoundButNotExported(t *testing.T) {
	m := newMachine(t)
	sdk := m.dir(defaultAndroidSDK)

	result := m.run(checkAndroidHome)
	assertStatus(t, result, StatusWarn)
	assertContains(t, result.Fix, "ANDROID_HOME")

	m.vars["ANDROID_HOME"] = sdk
	assertStatus(t, m.run(checkAndroidHome), StatusPass)
}

func TestAndroidSDKComponents(t *testing.T) {
	m := newMachine(t)
	m.vars["ANDROID_HOME"] = m.dir(defaultAndroidSDK)
	m.file(filepath.Join(defaultAndroidSDK, "emulator", "emulator"))
	m.dir(filepath.Join(defaultAndroidSDK, "build-tools", "34.0.0"))
	m.dir(filepath.Join(defaultAndroidSDK, "platforms", "android-36"))

	emulator := sdkBinary{name: "emulator", dir: "emulator", component: "Emulator"}
	assertStatus(t, m.run(emulator.check), StatusWarn)
	assertStatus(t, m.run(checkCmdlineTools), StatusFail)
	assertStatus(t, m.run(checkBuildTools), StatusFail)
	assertStatus(t, m.run(checkSDKPlatform), StatusPass)
	assertStatus(t, m.run(checkNDK), StatusWarn)
	assertStatus(t, m.run(checkAVD), StatusWarn)

	m.tools["emulator"] = "/sdk/emulator/emulator"
	m.file(filepath.Join(defaultAndroidSDK, "cmdline-tools", "latest", "bin", "sdkmanager"))
	m.dir(filepath.Join(defaultAndroidSDK, "build-tools", "36.1.0"))
	assertStatus(t, m.run(emulator.check), StatusPass)
	assertStatus(t, m.run(checkCmdlineTools), StatusPass)
	assertStatus(t, m.run(checkBuildTools), StatusPass)
}

func TestAndroidChecksSkipWithoutSDK(t *testing.T) {
	assertStatus(t, newMachine(t).run(checkBuildTools), StatusSkip)
}

func TestJDKVersion(t *testing.T) {
	m := newMachine(t)
	m.install("java", "java --version", "openjdk 21.0.4 2024-07-16")

	assertStatus(t, m.run(checkJDK), StatusWarn)
}

func TestGoToolOutsidePath(t *testing.T) {
	m := newMachine(t)
	m.install("go", "go env GOPATH", filepath.Join(m.home, "go"))
	tool := goTool{name: "gopls", install: "go install golang.org/x/tools/gopls@latest"}

	result := m.run(tool.check)
	assertStatus(t, result, StatusFail)
	assertContains(t, result.Fix, "go install")

	m.file("go/bin/gopls")
	assertStatus(t, m.run(tool.check), StatusWarn)
	assertStatus(t, m.run(checkGoBinOnPath), StatusWarn)

	m.vars["PATH"] = filepath.Join(m.home, "go", "bin")
	assertStatus(t, m.run(checkGoBinOnPath), StatusPass)
}

func TestClaudeCodeUpdateFixMatchesInstall(t *testing.T) {
	m := newMachine(t)
	m.install("claude", "claude --version", "2.1.267 (Claude Code)")
	m.releases.npm[claudeCodePackage] = Version{2, 1, 284}

	result := m.run(checkClaudeCode)

	assertStatus(t, result, StatusWarn)
	if result.Fix != "brew upgrade claude-code" {
		t.Errorf("fix = %q", result.Fix)
	}
}
