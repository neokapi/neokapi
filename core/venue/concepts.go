package venue

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// TermEdit holds the fields an ordinary term edit can set.
type TermEdit struct {
	Text         string   `json:"text"`
	Locale       string   `json:"locale"`
	Status       string   `json:"status"`
	PartOfSpeech string   `json:"part_of_speech,omitempty"`
	Gender       string   `json:"gender,omitempty"`
	Note         string   `json:"note,omitempty"`
	Forms        []string `json:"forms,omitempty"`
}

// ConceptInfo carries a concept and its project scope on the read API.
type ConceptInfo struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id,omitempty"`
	// Projects are the projects the concept is limited to, empty for a
	// concept of the whole workspace.
	Projects       []string          `json:"projects,omitempty"`
	Domain         string            `json:"domain"`
	Definition     string            `json:"definition"`
	DoNotTranslate bool              `json:"do_not_translate,omitempty"`
	Source         terms.TermSource  `json:"source,omitempty"`
	Advisory       bool              `json:"advisory,omitempty"`
	Terms          []terms.Term      `json:"terms"`
	Properties     map[string]string `json:"properties,omitempty"`
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
}

// EditedTerms applies editable fields and keeps each matching term's validity and competitor flag.
func EditedTerms(stored []terms.Term, edits []TermEdit) []terms.Term {
	previous := make(map[string]terms.Term, len(stored))
	for _, t := range stored {
		previous[terms.TermIdentity(t)] = t
	}
	result := make([]terms.Term, len(edits))
	for i, edit := range edits {
		locale := model.LocaleID(edit.Locale)
		key := terms.TermIdentity(terms.Term{Locale: locale, Text: edit.Text})
		t := previous[key]
		t.Text = edit.Text
		t.Locale = locale
		t.Status = model.TermStatus(edit.Status)
		t.PartOfSpeech = edit.PartOfSpeech
		t.Gender = edit.Gender
		t.Note = edit.Note
		t.Forms = edit.Forms
		if t.Status == "" {
			t.Status = model.TermApproved
		}
		result[i] = t
	}
	return result
}
