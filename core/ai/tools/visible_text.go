package tools

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/redaction"
)

// visibleText is the plain text of runs as a prompt that carries text alone
// reads it: each value a redact step withheld is drawn as its visible token
// ([REDACTED:Person]), where the block's own SourceText leaves nothing. A
// model that reads the token knows a value stood there, and a token it copies
// into its answer is restored with the block.
func visibleText(runs []model.Run) string {
	text, _ := redaction.VisibleText(runs)
	return text
}
