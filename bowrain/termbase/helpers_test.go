//go:build integration

package termbase_test

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
)

// softwareConcepts is the shared fixture used by the Postgres integration tests
// (mirrors the framework termbase test fixture). It lives here because test
// helpers don't cross package boundaries — without it, this package's
// integration tests don't compile.
func softwareConcepts() []termbase.Concept {
	return []termbase.Concept{
		{
			ID:         "c1",
			Domain:     "software",
			Definition: "To store data persistently",
			Terms: []termbase.Term{
				{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
				{Text: "Sauvegarder", Locale: model.LocaleFrench, Status: model.TermPreferred},
				{Text: "Speichern", Locale: model.LocaleGerman, Status: model.TermPreferred},
			},
		},
		{
			ID:         "c2",
			Domain:     "software",
			Definition: "To abort the current operation",
			Terms: []termbase.Term{
				{Text: "Cancel", Locale: model.LocaleEnglish, Status: model.TermPreferred},
				{Text: "Annuler", Locale: model.LocaleFrench, Status: model.TermPreferred},
				{Text: "Abbrechen", Locale: model.LocaleGerman, Status: model.TermPreferred},
			},
		},
		{
			ID:         "c3",
			Domain:     "software",
			Definition: "A repository for source code",
			Terms: []termbase.Term{
				{Text: "Repository", Locale: model.LocaleEnglish, Status: model.TermPreferred},
				{Text: "Repo", Locale: model.LocaleEnglish, Status: model.TermAdmitted},
				{Text: "Depot", Locale: model.LocaleFrench, Status: model.TermPreferred, Note: "Use depot, not repository"},
				{Text: "Repository", Locale: model.LocaleGerman, Status: model.TermPreferred},
			},
		},
	}
}
