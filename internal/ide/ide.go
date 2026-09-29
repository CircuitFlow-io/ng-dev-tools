// Package ide finds the editors and IDEs installed on this Mac and opens projects in them.
package ide

import "strings"

const (
	xcodeBundleID         = "com.apple.dt.Xcode"
	androidStudioBundleID = "com.google.android.studio"
)

// IDE is an installed editor or IDE application.
type IDE struct {
	Name     string
	Version  string
	BundleID string
	AppPath  string
}

// Label is the name with the version, e.g. "Xcode 26.6".
func (i IDE) Label() string {
	if i.Version == "" {
		return i.Name
	}
	return i.Name + " " + i.Version
}

func (i IDE) is(bundleID string) bool {
	return strings.HasPrefix(i.BundleID, bundleID)
}

// product is a supported IDE. Its bundle ID is a prefix, since preview and EAP builds append to it.
type product struct {
	name           string
	bundleIDPrefix string
}

var products = []product{
	{"Visual Studio Code", "com.microsoft.VSCode"},
	{"VSCodium", "com.vscodium"},
	{"Cursor", "com.todesktop.230313mzl4w4u92"},
	{"Windsurf", "com.exafunction.windsurf"},
	{"Zed", "dev.zed.Zed"},
	{"WebStorm", "com.jetbrains.WebStorm"},
	{"GoLand", "com.jetbrains.goland"},
	{"IntelliJ IDEA", "com.jetbrains.intellij"},
	{"PyCharm", "com.jetbrains.pycharm"},
	{"Rider", "com.jetbrains.rider"},
	{"Android Studio", androidStudioBundleID},
	{"Xcode", xcodeBundleID},
	{"Sublime Text", "com.sublimetext."},
	{"Nova", "com.panic.Nova"},
}

func productFor(bundleID string) (product, bool) {
	for _, p := range products {
		if strings.HasPrefix(bundleID, p.bundleIDPrefix) {
			return p, true
		}
	}
	return product{}, false
}
