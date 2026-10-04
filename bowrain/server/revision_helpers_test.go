package server

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// enSourceRevision is the revision of a plain-text source as the store stamps
// it for a project written in English: the basis a decision made on that
// source names.
func enSourceRevision(text string) string {
	return venue.SourceRevision(model.NewBlock("", text), "en")
}

// textRevision is the revision of a plain-text translation into locale: the
// revision a decision on it names.
func textRevision(locale model.LocaleID, text string) string {
	return model.RunsRevision(model.Variant(locale), []model.Run{model.TextR(text)})
}
