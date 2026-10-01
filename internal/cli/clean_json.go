package cli

import (
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup"
)

type cleanJSON struct {
	TotalBytes int64           `json:"totalBytes"`
	Items      []cleanItemJSON `json:"items"`
	Warnings   []string        `json:"warnings,omitempty"`
}

type cleanItemJSON struct {
	Category string `json:"category"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	// Safety is "safe" for things their tool regenerates, "review" for what you may want to keep.
	Safety    string    `json:"safety"`
	SizeBytes int64     `json:"sizeBytes"`
	Paths     []string  `json:"paths"`
	LastUsed  time.Time `json:"lastUsed,omitzero"`
	NeedsRoot bool      `json:"needsRoot"`
}

func toCleanJSON(result cleanup.ScanResult) cleanJSON {
	items := make([]cleanItemJSON, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, cleanItemJSON{
			Category:  item.Category.Key(),
			Title:     item.Title,
			Detail:    item.Detail,
			Safety:    safetyLabel(item.Safety),
			SizeBytes: item.Size,
			Paths:     item.Paths,
			LastUsed:  item.LastUsed,
			NeedsRoot: item.NeedsRoot,
		})
	}
	return cleanJSON{
		TotalBytes: cleanup.TotalSize(result.Items),
		Items:      items,
		Warnings:   mapSlice(result.Warnings, error.Error),
	}
}
