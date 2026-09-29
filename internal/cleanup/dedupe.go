package cleanup

import "path/filepath"

// dedupe makes sure every path belongs to at most one item, so sizes are not double counted
// and nothing is deleted twice. The broadest path wins; on an exact tie the earlier item keeps it.
// Items left with no paths and no remove command are dropped.
func dedupe(items []Item) []Item {
	claims := newPathClaims()
	for i := range items {
		for _, path := range items[i].Paths {
			claims.claim(path, i)
		}
	}

	kept := items[:0]
	for i, item := range items {
		item.Paths = claims.ownedBy(item.Paths, i)
		if len(item.Paths) == 0 && len(item.RemoveCommand) == 0 {
			continue
		}
		kept = append(kept, item)
	}
	return kept
}

type pathClaims struct {
	owner map[string]int
	// descendants counts claimed paths strictly below each directory, so releasing
	// descendants only walks the claims when there is something to release.
	descendants map[string]int
}

func newPathClaims() *pathClaims {
	return &pathClaims{owner: make(map[string]int), descendants: make(map[string]int)}
}

func (c *pathClaims) claim(path string, index int) {
	if c.isCovered(path) {
		return
	}
	if c.descendants[path] > 0 {
		c.releaseBelow(path)
	}
	c.owner[path] = index
	c.adjustAncestors(path, 1)
}

func (c *pathClaims) isCovered(path string) bool {
	for p := path; ; p = filepath.Dir(p) {
		if _, ok := c.owner[p]; ok {
			return true
		}
		if p == filepath.Dir(p) {
			return false
		}
	}
}

func (c *pathClaims) releaseBelow(dir string) {
	for claimed := range c.owner {
		if isWithin(claimed, dir) {
			delete(c.owner, claimed)
			c.adjustAncestors(claimed, -1)
		}
	}
}

func (c *pathClaims) adjustAncestors(path string, delta int) {
	for p := filepath.Dir(path); ; p = filepath.Dir(p) {
		c.descendants[p] += delta
		if p == filepath.Dir(p) {
			return
		}
	}
}

func (c *pathClaims) ownedBy(paths []string, index int) []string {
	var owned []string
	for _, path := range paths {
		if i, ok := c.owner[path]; ok && i == index {
			owned = append(owned, path)
		}
	}
	return owned
}
