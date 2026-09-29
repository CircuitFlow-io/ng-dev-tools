package tui

import (
	"errors"
	"fmt"
	"strings"
	"syscall"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	maxListedFailures   = 10
	categoryColumnWidth = 24
	fullDiskAccessTip   = "Tip: grant Full Disk Access to your terminal in System Settings › Privacy & Security, then run again."
)

func (m Model) summaryView() string {
	var sections []string
	switch {
	case m.results != nil:
		sections = append(sections, m.resultsSummary())
	case m.aborted:
		sections = append(sections, ui.Muted.Render("Cancelled. Nothing was deleted."))
	default:
		sections = append(sections, ui.Success.Render("✔ Your Mac is already tidy: nothing worth cleaning was found."))
	}
	if len(m.warnings) > 0 {
		sections = append(sections, warningsList(m.warnings))
	}
	return strings.Join(sections, "\n\n")
}

func (m Model) resultsSummary() string {
	succeeded, failed := splitResults(m.results)
	freed := cleanup.TotalSize(succeeded)

	headline := ui.Success.Bold(true).Render(fmt.Sprintf("✔ Freed %s", ui.Bytes(freed))) +
		ui.Muted.Render("  ("+ui.Count(len(succeeded), "item")+")")
	if m.cfg.Cleaner.DryRun {
		headline = ui.Title.Render(fmt.Sprintf("Dry run: would free %s", ui.Bytes(freed))) +
			ui.Muted.Render("  ("+ui.Count(len(succeeded), "item")+", nothing deleted)")
	}

	sections := []string{headline, categoryBreakdown(succeeded)}
	if m.stopping {
		sections = append(sections, ui.Warning.Render("Stopped early at your request."))
	}
	if len(failed) > 0 {
		sections = append(sections, m.failureList(failed))
	}
	if m.cfg.LogPath != "" {
		sections = append(sections, ui.Muted.Render("Log: "+m.cfg.LogPath))
	}
	return strings.Join(sections, "\n")
}

func splitResults(results []cleanup.CleanResult) (succeeded []cleanup.Item, failed []cleanup.CleanResult) {
	for _, r := range results {
		if r.Err != nil {
			failed = append(failed, r)
			continue
		}
		succeeded = append(succeeded, r.Item)
	}
	return succeeded, failed
}

func categoryBreakdown(items []cleanup.Item) string {
	var lines []string
	for _, category := range cleanup.AllCategories {
		var total int64
		for _, item := range items {
			if item.Category == category {
				total += item.Size
			}
		}
		if total > 0 {
			lines = append(lines, "  "+ui.PadRight(category.String(), categoryColumnWidth)+ui.Bold.Render(ui.Bytes(total)))
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) failureList(failed []cleanup.CleanResult) string {
	lines := []string{"", ui.Danger.Render(fmt.Sprintf("✘ %d item(s) could not be removed:", len(failed)))}
	needsFullDiskAccess := false
	for i, r := range failed {
		needsFullDiskAccess = needsFullDiskAccess || errors.Is(r.Err, syscall.EPERM)
		if i == maxListedFailures {
			lines = append(lines, ui.Muted.Render(fmt.Sprintf("  … and %d more (see log)", len(failed)-maxListedFailures)))
			break
		}
		lines = append(lines, fmt.Sprintf("  • %s: %s", r.Item.Title, ui.Muted.Render(ui.Truncate(r.Err.Error(), m.width-len(r.Item.Title)-8))))
	}
	if m.sudoErr != nil {
		lines = append(lines, ui.Warning.Render("  Administrator access was not granted, so root-owned items were skipped."))
	}
	if needsFullDiskAccess {
		lines = append(lines, ui.Muted.Render("  "+fullDiskAccessTip))
	}
	return strings.Join(lines, "\n")
}

func warningsList(warnings []error) string {
	lines := []string{ui.Warning.Render("⚠ Some sources were skipped:")}
	for _, w := range warnings {
		lines = append(lines, ui.Muted.Render("  • "+w.Error()))
	}
	return strings.Join(lines, "\n")
}
