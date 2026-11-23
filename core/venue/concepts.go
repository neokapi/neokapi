package venue

import (
	"github.com/neokapi/neokapi/core/graph"
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

// TermInfo carries a term's wording, lifecycle and scope in a concept response.
type TermInfo struct {
	Text           string          `json:"text"`
	Locale         string          `json:"locale"`
	Status         string          `json:"status"`
	PartOfSpeech   string          `json:"part_of_speech,omitempty"`
	Gender         string          `json:"gender,omitempty"`
	Note           string          `json:"note,omitempty"`
	Forms          []string        `json:"forms,omitempty"`
	CompetitorTerm bool            `json:"competitor_term,omitempty"`
	Validity       *graph.Validity `json:"validity,omitempty"`
}

// TermInfos projects all term fields onto the read API.
func TermInfos(ts []terms.Term) []TermInfo {
	out := make([]TermInfo, len(ts))
	for i, t := range ts {
		out[i] = TermInfo{
			Text: t.Text, Locale: string(t.Locale), Status: string(t.Status),
			PartOfSpeech: t.PartOfSpeech, Gender: t.Gender, Note: t.Note, Forms: t.Forms,
			CompetitorTerm: t.CompetitorTerm, Validity: t.Validity,
		}
	}
	return out
}

// Term reconstructs the stored term with its governance fields.
func (t TermInfo) Term() terms.Term {
	return terms.Term{
		Text: t.Text, Locale: model.LocaleID(t.Locale), Status: model.TermStatus(t.Status),
		PartOfSpeech: t.PartOfSpeech, Gender: t.Gender, Note: t.Note, Forms: t.Forms,
		CompetitorTerm: t.CompetitorTerm, Validity: t.Validity,
	}
}

// TermEdits selects the fields an ordinary create or update may send.
func TermEdits(ts []TermInfo) []TermEdit {
	if ts == nil {
		return nil
	}
	edits := make([]TermEdit, len(ts))
	for i, t := range ts {
		edits[i] = TermEdit{
			Text: t.Text, Locale: t.Locale, Status: t.Status,
			PartOfSpeech: t.PartOfSpeech, Gender: t.Gender, Note: t.Note, Forms: t.Forms,
		}
	}
	return edits
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
	Terms          []TermInfo        `json:"terms"`
	Properties     map[string]string `json:"properties,omitempty"`
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
	// Revision is the stored concept's revision when this was read. An
	// update names it as its base revision.
	Revision string `json:"revision,omitempty"`
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
