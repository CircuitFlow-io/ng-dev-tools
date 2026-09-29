package cli

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/nasserghiasi/ng-dev-tools/internal/cleanup"
	"github.com/nasserghiasi/ng-dev-tools/internal/ui"
)

// printReport lists findings as plain text. It is used when output is not a terminal,
// for example when piped, and never deletes anything.
func printReport(ctx context.Context, out io.Writer, scanner cleanup.Scanner) error {
	result := scanner.Scan(ctx, nil)
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CATEGORY\tSIZE\tSAFETY\tITEM\tLOCATION")
	for _, item := range result.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", item.Category.Key(), ui.Bytes(item.Size), safetyLabel(item.Safety), item.Title, item.Detail)
	}
	fmt.Fprintf(w, "\nTOTAL\t%s\t\t%d items\t\n", ui.Bytes(cleanup.TotalSize(result.Items)), len(result.Items))
	for _, warning := range result.Warnings {
		fmt.Fprintf(w, "WARNING\t\t\t%s\t\n", warning)
	}
	return w.Flush()
}

func safetyLabel(s cleanup.Safety) string {
	if s == cleanup.SafetyReview {
		return "review"
	}
	return "safe"
}
