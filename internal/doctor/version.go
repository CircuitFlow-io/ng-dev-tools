package doctor

import (
	"cmp"
	"fmt"
	"regexp"
	"strconv"
)

var versionPattern = regexp.MustCompile(`(\d+)(?:\.(\d+))?(?:\.(\d+))?`)

// Version is a major.minor.patch release number.
type Version struct {
	Major, Minor, Patch int
}

// ParseVersion reads the first dotted number in text, such as a tool's --version output.
func ParseVersion(text string) (Version, bool) {
	match := versionPattern.FindStringSubmatch(text)
	if match == nil {
		return Version{}, false
	}
	return Version{Major: atoi(match[1]), Minor: atoi(match[2]), Patch: atoi(match[3])}, true
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// Compare orders versions like cmp.Compare.
func (v Version) Compare(other Version) int {
	return cmp.Or(cmp.Compare(v.Major, other.Major), cmp.Compare(v.Minor, other.Minor), cmp.Compare(v.Patch, other.Patch))
}

// Less reports whether v is older than other.
func (v Version) Less(other Version) bool {
	return v.Compare(other) < 0
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}
