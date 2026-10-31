package redaction

import "github.com/neokapi/neokapi/core/model"

// ShowTokens returns runs with each redaction placeholder replaced by a text
// run holding its visible token (Disp), in every plural form and select case
// too. Every other run is kept as it is. A writer that has no placeholder
// form of its own then writes the token as text, and [RestoreText] puts the
// original back wherever the token survives.
func ShowTokens(runs []model.Run) []model.Run {
	var tokens []string
	return showTokens(runs, &tokens)
}
