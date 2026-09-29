package todos

import (
	"bufio"
	"os"
	"path/filepath"
)

// SourceLine is a numbered line of a file.
type SourceLine struct {
	Number int
	Text   string
}

// Surroundings reads the item's line with up to before lines above it and after lines below.
func Surroundings(item Item, before, after int) ([]SourceLine, error) {
	f, err := os.Open(filepath.Join(item.Dir, item.File))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	first, last := max(1, item.Line-before), item.Line+after
	var lines []SourceLine
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxSourceLineBytes)
	for n := 1; scanner.Scan() && n <= last; n++ {
		if n >= first {
			lines = append(lines, SourceLine{Number: n, Text: scanner.Text()})
		}
	}
	return lines, scanner.Err()
}

// maxSourceLineBytes lets a long line be read rather than stop the read.
const maxSourceLineBytes = 1 << 20
