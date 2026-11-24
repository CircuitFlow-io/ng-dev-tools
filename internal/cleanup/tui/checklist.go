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
	groupIndent   = "  "
)

type rowKind int

const (
	rowCategory rowKind = iota
	rowGroup
	rowItem
)

// row is one line of the checklist: a category header, a group header or an item.
type row struct {
	kind     rowKind
	category cleanup.Category
	group    string
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

// newChecklist preselects items that are safe to remove. Items must be grouped by category,
// and items sharing a Group must be adjacent.
func newChecklist(items []cleanup.Item) checklist {
	c := checklist{items: items, selected: make([]bool, len(items))}
	for i, item := range items {
		c.selected[i] = item.Safety == cleanup.SafetySafe
		newCategory := i == 0 || items[i-1].Category != item.Category
		if newCategory {
			c.rows = append(c.rows, row{kind: rowCategory, category: item.Category})
		}
		if item.Group != "" && (newCategory || items[i-1].Group != item.Group) {
			c.rows = append(c.rows, row{kind: rowGroup, category: item.Category, group: item.Group})
		}
		c.rows = append(c.rows, row{kind: rowItem, category: item.Category, group: item.Group, item: i})
	}
	c.cursor = ui.NewListCursor(len(c.rows), 1)
	return c
}

func (c *checklist) resize(width, height int) {
	c.width = width
	c.cursor.Resize(height)
}

// toggle flips the item under the cursor, or every item under a header.
func (c *checklist) toggle() {
	current := c.rows[c.cursor.Index]
	switch current.kind {
	case rowCategory:
		c.toggleCategory(current.category)
	case rowGroup:
		c.toggleGroup(current.category, current.group)
	case rowItem:
		c.selected[current.item] = !c.selected[current.item]
	}
}

func (c *checklist) toggleCurrentCategory() {
	c.toggleCategory(c.rows[c.cursor.Index].category)
}

// toggleCurrentGroup flips the group the cursor is in; outside a group it flips the single item.
func (c *checklist) toggleCurrentGroup() {
	current := c.rows[c.cursor.Index]
	if current.group == "" {
		c.toggle()
		return
	}
	c.toggleGroup(current.category, current.group)
}

func (c *checklist) toggleCategory(category cleanup.Category) {
	c.toggleWhere(func(item cleanup.Item) bool { return item.Category == category })
}

func (c *checklist) toggleGroup(category cleanup.Category, group string) {
	c.toggleWhere(func(item cleanup.Item) bool { return item.Category == category && item.Group == group })
}

// toggleWhere selects every matching item, or clears them if all were already selected.
func (c *checklist) toggleWhere(matches func(cleanup.Item) bool) {
	allSelected := true
	for i, item := range c.items {
		if matches(item) && !c.selected[i] {
			allSelected = false
			break
		}
	}
	for i, item := range c.items {
		if matches(item) {
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

func (c checklist) currentRow() row {
	return c.rows[c.cursor.Index]
}

func (c checklist) currentItem() (cleanup.Item, bool) {
	current := c.currentRow()
	if current.kind != rowItem {
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
	switch r.kind {
	case rowCategory:
		content = c.renderCategory(p, r.category)
	case rowGroup:
		content = c.renderGroup(p, r)
	case rowItem:
		content = c.renderItem(p, r, now)
	}
	return p.Fill(p.Cursor()+content, c.width)
}

func (c checklist) renderCategory(p ui.RowPainter, category cleanup.Category) string {
	stats := c.stats(func(item cleanup.Item) bool { return item.Category == category })
	return p.Paint(ui.Heading, category.String()) + p.Paint(ui.Muted, stats)
}

func (c checklist) renderGroup(p ui.RowPainter, r row) string {
	stats := c.stats(func(item cleanup.Item) bool { return item.Category == r.category && item.Group == r.group })
	return p.Paint(lipgloss.NewStyle(), groupIndent) + p.Paint(ui.Bold, r.group) + p.Paint(ui.Muted, stats)
}

// stats summarises matching items as "  3 items · 1.2 GB · 400 MB selected".
func (c checklist) stats(matches func(cleanup.Item) bool) string {
	var count int
	var total, chosen int64
	for i, item := range c.items {
		if !matches(item) {
			continue
		}
		count++
		total += item.Size
		if c.selected[i] {
			chosen += item.Size
		}
	}
	return fmt.Sprintf("  %s · %s · %s selected", ui.Count(count, "item"), ui.Bytes(total), ui.Bytes(chosen))
}

func (c checklist) renderItem(p ui.RowPainter, r row, now time.Time) string {
	item := c.items[r.item]
	indent := ""
	if r.group != "" {
		indent = groupIndent
	}
	box := p.Paint(ui.Muted, indent+"[ ] ")
	if c.selected[r.item] {
		box = p.Paint(ui.Success, indent+"[✓] ")
	}
	titleWidth := max(minTitleWidth, c.width-cursorWidth-len(indent)-checkboxWidth-sizeWidth-tagsWidth)
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
