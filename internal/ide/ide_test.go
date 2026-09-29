package ide

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos/macostest"
)

const infoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>%s</string>
<key>CFBundleShortVersionString</key><string>%s</string>
</dict></plist>`

func fakeApp(t *testing.T, path, bundleID, version string) {
	t.Helper()
	contents := filepath.Join(path, "Contents")
	if err := os.MkdirAll(contents, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := []byte(fmt.Sprintf(infoPlist, bundleID, version))
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), plist, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestDetect(t *testing.T) {
	system, user := t.TempDir(), t.TempDir()
	fakeApp(t, filepath.Join(system, "Cursor.app"), "com.todesktop.230313mzl4w4u92", "3.22.12")
	fakeApp(t, filepath.Join(system, "Xcode-26.app"), "com.apple.dt.Xcode", "26.6")
	fakeApp(t, filepath.Join(system, "Xcode.app"), "com.apple.dt.Xcode", "27.0")
	fakeApp(t, filepath.Join(system, "Safari.app"), "com.apple.Safari", "19.0")
	fakeApp(t, filepath.Join(user, "WebStorm.app"), "com.jetbrains.WebStorm-EAP", "EAP WS-263")
	fakeApp(t, filepath.Join(user, "JetBrains Toolbox", "GoLand.app"), "com.jetbrains.goland", "2026.2")
	mkdir(t, filepath.Join(system, "Broken.app"))

	var labels []string
	for _, found := range Detect([]string{system, user}) {
		labels = append(labels, found.Label())
	}

	want := []string{"Cursor 3.22.12", "GoLand 2026.2", "WebStorm EAP WS-263", "Xcode 27.0", "Xcode 26.6"}
	if !slices.Equal(labels, want) {
		t.Errorf("Detect = %v, want %v", labels, want)
	}
}

func TestCompareVersionsIsNumeric(t *testing.T) {
	if compareVersions("9.4", "16.2") >= 0 || compareVersions("27.0", "26.6") <= 0 || compareVersions("1.2", "1.2") != 0 {
		t.Error("versions compared as text")
	}
}

func TestTarget(t *testing.T) {
	xcode := IDE{BundleID: "com.apple.dt.Xcode"}
	studio := IDE{BundleID: "com.google.android.studio"}
	zed := IDE{BundleID: "dev.zed.Zed"}

	rn := t.TempDir()
	mkdir(t, filepath.Join(rn, "ios", "App.xcodeproj"))
	mkdir(t, filepath.Join(rn, "ios", "App.xcworkspace"))
	mkdir(t, filepath.Join(rn, "android"))

	native := t.TempDir()
	mkdir(t, filepath.Join(native, "android"))
	if err := os.WriteFile(filepath.Join(native, "settings.gradle.kts"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		ide     IDE
		project string
		want    string
	}{
		{"xcode prefers the workspace in ios/", xcode, rn, filepath.Join(rn, "ios", "App.xcworkspace")},
		{"xcode falls back to the folder", xcode, native, native},
		{"android studio opens android/", studio, rn, filepath.Join(rn, "android")},
		{"android studio keeps a gradle root", studio, native, native},
		{"other IDEs open the folder", zed, rn, rn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Target(tt.ide, tt.project); got != tt.want {
				t.Errorf("Target = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpenUsesTheAppBundle(t *testing.T) {
	runner := &macostest.Runner{}
	cursor := IDE{Name: "Cursor", AppPath: "/Applications/Cursor.app"}

	if err := Open(context.Background(), runner, cursor, "/p/memorit"); err != nil {
		t.Fatal(err)
	}
	if calls := runner.Calls(); len(calls) != 1 || calls[0] != "open -a /Applications/Cursor.app /p/memorit" {
		t.Errorf("calls = %q", calls)
	}
}
