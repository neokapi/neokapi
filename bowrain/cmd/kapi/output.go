package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/gokapi/gokapi/flow"
)

// TableFormattable is implemented by collector result data that can
// render itself as an aligned text table.
type TableFormattable interface {
	FormatTable(w io.Writer)
}

// formatOutput writes the collector result to stdout.
// In JSON mode it marshals result.Data; in text mode it calls FormatTable
// if available, falling back to JSON.
func formatOutput(jsonMode bool, result flow.CollectorResult) error {
	if jsonMode {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result.Data)
	}

	if ft, ok := result.Data.(TableFormattable); ok {
		ft.FormatTable(os.Stdout)
		return nil
	}

	// Fallback: pretty-print JSON.
	data, err := json.MarshalIndent(result.Data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
