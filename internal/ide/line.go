package ide

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/fsx"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

const (
	jetBrainsBundleIDPrefix = "com.jetbrains."
	zedBundleID             = "dev.zed.Zed"
	sublimeBundleIDPrefix   = "com.sublimetext."
)

// vscodeCLIs are the command-line tools VS Code and its forks bundle, by bundle ID prefix. They
// open a folder and jump to file:line in one call.
var vscodeCLIs = map[string]string{
	"com.microsoft.VSCode":          "code",
	"com.vscodium":                  "codium",
	"com.todesktop.230313mzl4w4u92": "cursor",
	"com.exafunction.windsurf":      "windsurf",
}

// OpenAt opens file at line in ide, inside project when the IDE takes both. An IDE with no way
// to jump to a line gets the file.
func OpenAt(ctx context.Context, runner macos.Runner, ide IDE, project, file string, line int) error {
	name, args := LineCommand(ide, project, file, line)
	_, err := runner.Run(ctx, name, args...)
	return err
}

// LineCommand is the command that opens file at line in ide.
func LineCommand(ide IDE, project, file string, line int) (string, []string) {
	n := strconv.Itoa(line)
	at := file + ":" + n
	if cli, ok := bundledCLI(ide); ok {
		if ide.is(zedBundleID) {
			return cli, []string{project, at}
		}
		if ide.is(sublimeBundleIDPrefix) {
			return cli, []string{at}
		}
		return cli, []string{project, "-g", at}
	}
	switch {
	case ide.is(xcodeBundleID):
		return "xed", []string{"--line", n, file}
	case ide.is(jetBrainsBundleIDPrefix), ide.is(androidStudioBundleID):
		return "open", []string{"-na", ide.AppPath, "--args", "--line", n, file}
	}
	return "open", []string{"-a", ide.AppPath, file}
}

// bundledCLI is the IDE's own command-line tool, when it has one that takes file:line.
func bundledCLI(ide IDE) (string, bool) {
	var path string
	switch {
	case ide.is(zedBundleID):
		path = filepath.Join(ide.AppPath, "Contents", "MacOS", "cli")
	case ide.is(sublimeBundleIDPrefix):
		path = filepath.Join(ide.AppPath, "Contents", "SharedSupport", "bin", "subl")
	default:
		name, ok := vscodeCLI(ide)
		if !ok {
			return "", false
		}
		path = filepath.Join(ide.AppPath, "Contents", "Resources", "app", "bin", name)
	}
	return path, fsx.Exists(path)
}

func vscodeCLI(ide IDE) (string, bool) {
	for prefix, name := range vscodeCLIs {
		if strings.HasPrefix(ide.BundleID, prefix) {
			return name, true
		}
	}
	return "", false
}
