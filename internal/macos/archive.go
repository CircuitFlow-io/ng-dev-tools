package macos

import (
	"os"
	"path/filepath"
	"time"

	"howett.net/plist"
)

// ArchiveInfo is the part of an Xcode archive's Info.plist ngt reads.
type ArchiveInfo struct {
	Name         string    `plist:"Name"`
	CreationDate time.Time `plist:"CreationDate"`
	App          struct {
		Version string `plist:"CFBundleShortVersionString"`
		Build   string `plist:"CFBundleVersion"`
	} `plist:"ApplicationProperties"`
}

// ReadArchive reads the Info.plist of an .xcarchive bundle.
func ReadArchive(archivePath string) (ArchiveInfo, error) {
	f, err := os.Open(filepath.Join(archivePath, "Info.plist"))
	if err != nil {
		return ArchiveInfo{}, err
	}
	defer f.Close()

	var info ArchiveInfo
	if err := plist.NewDecoder(f).Decode(&info); err != nil {
		return ArchiveInfo{}, err
	}
	return info, nil
}
