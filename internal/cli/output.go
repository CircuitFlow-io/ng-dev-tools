package cli

import (
	"encoding/json"
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const jsonFlag = "json"

// outputMode is how a command shows its result.
type outputMode int

const (
	// outputTUI is the interactive screen, when stdout is a terminal.
	outputTUI outputMode = iota
	// outputText is a plain table, when stdout is piped or redirected.
	outputText
	// outputJSON is everything as one JSON object, for scripts and AI agents.
	outputJSON
)

func addJSONFlag(root *cobra.Command) {
	root.PersistentFlags().Bool(jsonFlag, false, "print the result as JSON instead of the interactive screen or table (for scripts and AI agents)")
}

func resolveOutput(cmd *cobra.Command) outputMode {
	if asJSON, _ := cmd.Flags().GetBool(jsonFlag); asJSON {
		return outputJSON
	}
	if term.IsTerminal(int(os.Stdout.Fd())) {
		return outputTUI
	}
	return outputText
}

// noticeWriter is where a command prints side remarks, such as an unreadable settings file, so
// they never break the JSON document on stdout.
func noticeWriter(cmd *cobra.Command, mode outputMode) io.Writer {
	if mode == outputJSON {
		return cmd.ErrOrStderr()
	}
	return cmd.OutOrStdout()
}

func writeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// errorMessages turns per-path failures into text for JSON, or nil when there are none.
func errorMessages(errs map[string]error) map[string]string {
	if len(errs) == 0 {
		return nil
	}
	messages := make(map[string]string, len(errs))
	for path, err := range errs {
		messages[path] = err.Error()
	}
	return messages
}

// mapSlice converts each element with convert, keeping nil as nil so empty lists are omitted.
func mapSlice[T, V any](items []T, convert func(T) V) []V {
	if len(items) == 0 {
		return nil
	}
	views := make([]V, len(items))
	for i, item := range items {
		views[i] = convert(item)
	}
	return views
}

// nonNil turns a nil list into an empty one, for top-level lists that should always be arrays.
func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
