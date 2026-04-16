package sievepen

import (
	"encoding/json"

	"github.com/neokapi/neokapi/core/model"
)

// isPlainTextRuns reports whether the Run sequence carries only a
// single TextRun — the hot-path shape for TMX bulk imports. When
// true the variant can be persisted as raw text without a JSON
// wrapper.
func isPlainTextRuns(runs []model.Run) bool {
	return len(runs) == 1 && runs[0].Text != nil
}

// decodeVariantRuns parses the `coded` column of tm_variants back
// into a Run sequence. A leading '[' indicates a JSON-encoded []Run;
// anything else is treated as plain text and materialised as a
// single TextRun.
func decodeVariantRuns(coded string) []model.Run {
	if len(coded) > 0 && coded[0] == '[' {
		var runs []model.Run
		if err := json.Unmarshal([]byte(coded), &runs); err == nil {
			return runs
		}
	}
	return []model.Run{{Text: &model.TextRun{Text: coded}}}
}
