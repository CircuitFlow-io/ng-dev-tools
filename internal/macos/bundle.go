package macos

import (
	"os"
	"path/filepath"

	"howett.net/plist"
)

// BundleInfo is the part of an application bundle's Info.plist ngt reads.
type BundleInfo struct {
	Identifier string `plist:"CFBundleIdentifier"`
	Version    string `plist:"CFBundleShortVersionString"`
}

// ReadBundle reads an application bundle's Info.plist.
func ReadBundle(appPath string) (BundleInfo, error) {
	f, err := os.Open(filepath.Join(appPath, "Contents", "Info.plist"))
	if err != nil {
		return BundleInfo{}, err
	}
	defer f.Close()

	var info BundleInfo
	if err := plist.NewDecoder(f).Decode(&info); err != nil {
		return BundleInfo{}, err
	}
	return info, nil
}

// BundleID reads CFBundleIdentifier from an application bundle's Info.plist.
func BundleID(appPath string) (string, error) {
	info, err := ReadBundle(appPath)
	return info.Identifier, err
}
