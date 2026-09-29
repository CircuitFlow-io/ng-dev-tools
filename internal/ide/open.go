package ide

import (
	"context"
	"os"
	"path/filepath"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/fsx"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

var (
	xcodeProjectPatterns = []string{"*.xcworkspace", "*.xcodeproj"}
	gradleBuildFiles     = []string{"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"}
)

// Open opens project in ide.
func Open(ctx context.Context, runner macos.Runner, ide IDE, project string) error {
	_, err := runner.Run(ctx, "open", "-a", ide.AppPath, Target(ide, project))
	return err
}

// Target is what to hand ide for project: Xcode gets the workspace or project file, including a
// React Native app's ios/ one, and Android Studio gets a React Native app's android/ folder.
func Target(ide IDE, project string) string {
	switch {
	case ide.is(xcodeBundleID):
		return xcodeTarget(project)
	case ide.is(androidStudioBundleID):
		return androidTarget(project)
	default:
		return project
	}
}

func xcodeTarget(project string) string {
	for _, dir := range []string{project, filepath.Join(project, "ios")} {
		for _, pattern := range xcodeProjectPatterns {
			if matches, _ := filepath.Glob(filepath.Join(dir, pattern)); len(matches) > 0 {
				return matches[0]
			}
		}
	}
	return project
}

func androidTarget(project string) string {
	for _, name := range gradleBuildFiles {
		if fsx.Exists(filepath.Join(project, name)) {
			return project
		}
	}
	android := filepath.Join(project, "android")
	if info, err := os.Stat(android); err == nil && info.IsDir() {
		return android
	}
	return project
}
