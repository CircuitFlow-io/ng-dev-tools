package tui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	cursorWidth   = 2
	checkboxWidth = 4
	sizeWidth     = 10
	tagsWidth     = 24
	minTitleWidth = 20
)

// row is one line of the checklist: either a category header or an item.
type row struct {
	isHeader bool
	category cleanup.Category
	item     int
}

// checklist is a scrollable, grouped multi-select list of scan results.
type checklist struct {
	items    []cleanup.Item
	selected []bool
	rows     []row
	cursor   ui.ListCursor
	width    int
	// highlight is the background of the row under the cursor.
	highlight color.Color
}

// newChecklist preselects items that are safe to remove. Items must be grouped by category.
func newChecklist(items []cleanup.Item) checklist {
	c := checklist{items: items, selected: make([]bool, len(items))}
	for i, item := range items {
		c.selected[i] = item.Safety == cleanup.SafetySafe
		if i == 0 || items[i-1].Category != item.Category {
			c.rows = append(c.rows, row{isHeader: true, category: item.Category})
		}
		c.rows = append(c.rows, row{category: item.Category, item: i})
	}
	c.cursor = ui.NewListCursor(len(c.rows), 1)
	return c
}

func (c *checklist) resize(width, height int) {
	c.width = width
	c.cursor.Resize(height)
}

// toggle flips the item under the cursor, or the whole category when on a header.
func (c *checklist) toggle() {
	current := c.rows[c.cursor.Index]
	if current.isHeader {
		c.toggleCategory(current.category)
		return
	}
	c.selected[current.item] = !c.selected[current.item]
}

func (c *checklist) toggleCurrentCategory() {
	c.toggleCategory(c.rows[c.cursor.Index].category)
}

// toggleCategory selects every item in the category, or clears them if all were selected.
func (c *checklist) toggleCategory(category cleanup.Category) {
	allSelected := true
	for i, item := range c.items {
		if item.Category == category && !c.selected[i] {
			allSelected = false
			break
		}
	}
	for i, item := range c.items {
		if item.Category == category {
			c.selected[i] = !allSelected
		}
	}
}

func (c *checklist) setAll(selected bool) {
	for i := range c.selected {
		c.selected[i] = selected
	}
}

func (c checklist) selectedItems() []cleanup.Item {
	var items []cleanup.Item
	for i, item := range c.items {
		if c.selected[i] {
			items = append(items, item)
		}
	}
	return items
}

func (c checklist) currentItem() (cleanup.Item, bool) {
	current := c.rows[c.cursor.Index]
	if current.isHeader {
		return cleanup.Item{}, false
	}
	return c.items[current.item], true
}

func (c checklist) view(now time.Time) string {
	start, end := c.cursor.Visible()
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, c.renderRow(i, now))
	}
	return strings.Join(lines, "\n")
}

func (c checklist) renderRow(index int, now time.Time) string {
	r := c.rows[index]
	p := ui.NewRowPainter(index == c.cursor.Index, c.highlight)
	var content string
	if r.isHeader {
		content = c.renderHeader(p, r.category)
	} else {
		content = c.renderItem(p, r.item, now)
	}
	return p.Fill(p.Cursor()+content, c.width)
}

func (c checklist) renderHeader(p ui.RowPainter, category cleanup.Category) string {
	var count int
	var total, chosen int64
	for i, item := range c.items {
		if item.Category != category {
			continue
		}
		count++
		total += item.Size
		if c.selected[i] {
			chosen += item.Size
		}
	}
	stats := fmt.Sprintf("  %s · %s · %s selected", ui.Count(count, "item"), ui.Bytes(total), ui.Bytes(chosen))
	return p.Paint(ui.Heading, category.String()) + p.Paint(ui.Muted, stats)
}

func (c checklist) renderItem(p ui.RowPainter, index int, now time.Time) string {
	item := c.items[index]
	box := p.Paint(ui.Muted, "[ ] ")
	if c.selected[index] {
		box = p.Paint(ui.Success, "[✓] ")
	}
	titleWidth := max(minTitleWidth, c.width-cursorWidth-checkboxWidth-sizeWidth-tagsWidth)
	titleStyle := lipgloss.NewStyle().Width(titleWidth)
	if p.IsHighlighted() {
		titleStyle = titleStyle.Bold(true)
	}
	title := p.Paint(titleStyle, ui.Truncate(item.Title, titleWidth-1))
	size := p.Paint(ui.Bold.Width(sizeWidth).Align(lipgloss.Right), ui.Bytes(item.Size))
	return box + title + size + p.Paint(lipgloss.NewStyle(), "  ") + itemTags(p, item, now)
}

func itemTags(p ui.RowPainter, item cleanup.Item, now time.Time) string {
	var tags []string
	if item.Safety == cleanup.SafetyReview {
		tags = append(tags, p.Paint(ui.Warning, "review"))
	}
	if item.NeedsRoot {
		tags = append(tags, p.Paint(ui.Danger, "sudo"))
	}
	if !item.LastUsed.IsZero() {
		tags = append(tags, p.Paint(ui.Muted, ui.Age(now, item.LastUsed)+" ago"))
	}
	return strings.Join(tags, p.Paint(lipgloss.NewStyle(), " "))
}
