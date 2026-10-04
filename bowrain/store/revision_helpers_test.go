package store

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// nbTextRevision is the revision of a plain-text Norwegian translation: the
// revision a decision on it names.
func nbTextRevision(text string) string {
	return model.RunsRevision(model.Variant("nb"), []model.Run{model.TextR(text)})
}

// sourceRevision is the revision of a plain-text source as the store stamps it
// for a project written in English (createTestProject): the basis a decision
// made on that source names.
func sourceRevision(text string) string {
	return venue.SourceRevision(blockWithText("", text), model.LocaleEnglish)
}

// linkRuns is a source that links its text to href: two blocks built from it
// with different targets differ in an inline code alone.
func linkRuns(text, href string) []model.Run {
	return []model.Run{
		model.TextR("Read "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link", Data: `<a href="` + href + `">`}),
		model.TextR(text),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "link", Data: "</a>"}),
	}
}

// linkedBlock is a translatable block whose source is linkRuns.
func linkedBlock(id, text, href string) *model.Block {
	b := &model.Block{ID: id, Translatable: true}
	b.SetSourceRuns(linkRuns(text, href))
	return b
}

// linkedSourceRevision is the revision of linkRuns as the store stamps it.
func linkedSourceRevision(text, href string) string {
	return venue.SourceRevision(linkedBlock("", text, href), model.LocaleEnglish)
}
