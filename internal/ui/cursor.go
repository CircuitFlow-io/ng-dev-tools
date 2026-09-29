package ui

// ListCursor tracks the cursor and scroll position of a list taller than its viewport.
type ListCursor struct {
	Index  int
	offset int
	height int
	length int
}

// NewListCursor places the cursor at index in a list of length rows, scrolled to the top.
// Call Resize before rendering.
func NewListCursor(length, index int) ListCursor {
	return ListCursor{Index: clamp(index, length), length: length, height: 1}
}

// Resize sets how many rows are visible at once.
func (c *ListCursor) Resize(height int) {
	c.height = max(height, 1)
	c.scrollToCursor()
}

// Move moves the cursor by delta rows, stopping at either end.
func (c *ListCursor) Move(delta int) {
	c.Index = clamp(c.Index+delta, c.length)
	c.scrollToCursor()
}

// MoveTo puts the cursor on row index.
func (c *ListCursor) MoveTo(index int) {
	c.Move(index - c.Index)
}

// HandleKey applies a navigation key and reports whether it was one.
func (c *ListCursor) HandleKey(key string) bool {
	switch key {
	case "up", "k":
		c.Move(-1)
	case "down", "j":
		c.Move(1)
	case "pgup", "ctrl+u":
		c.Move(-c.height)
	case "pgdown", "ctrl+d":
		c.Move(c.height)
	case "home", "g":
		c.MoveTo(0)
	case "end", "G":
		c.MoveTo(c.length - 1)
	default:
		return false
	}
	return true
}

// Visible returns the half-open range of rows in the viewport.
func (c ListCursor) Visible() (start, end int) {
	return c.offset, min(c.length, c.offset+c.height)
}

func (c *ListCursor) scrollToCursor() {
	if c.Index < c.offset {
		c.offset = c.Index
	}
	if c.Index >= c.offset+c.height {
		c.offset = c.Index - c.height + 1
	}
}

func clamp(index, length int) int {
	return max(0, min(length-1, index))
}
