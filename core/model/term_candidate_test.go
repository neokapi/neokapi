package model_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

func TestTermCandidateAnnotation_AnnotationType(t *testing.T) {
	tc := &model.TermCandidateAnnotation{
		Text:            "Dashboard",
		Definition:      "Main overview screen",
		Category:        model.TermCategoryUI,
		Translatability: model.TranslatabilityConsistent,
		Confidence:      0.92,
		Position:        model.RunRange{StartOffset: 5, EndOffset: 14},
		Locale:          "en-US",
		Source:          model.ExtractionSourceLLM,
		Status:          model.CandidateStatusPending,
	}
	assert.Equal(t, "term-candidate", tc.AnnotationType())
}

func TestTermCandidateAnnotation_ManualSource(t *testing.T) {
	tc := &model.TermCandidateAnnotation{
		Text:   "Workflow",
		Source: model.ExtractionSourceManual,
		Status: model.CandidateStatusPending,
	}
	assert.Equal(t, model.ExtractionSourceManual, tc.Source)
	assert.Equal(t, model.CandidateStatusPending, tc.Status)
}

func TestTermCandidateAnnotation_RegisteredInRegistry(t *testing.T) {
	ann, ok := model.NewAnnotation("term-candidate")
	assert.True(t, ok)
	assert.NotNil(t, ann)
	assert.Equal(t, "term-candidate", ann.AnnotationType())
}

func TestEntityAnnotation_RegisteredInRegistry(t *testing.T) {
	ann, ok := model.NewAnnotation("entity")
	assert.True(t, ok)
	assert.NotNil(t, ann)
	assert.Equal(t, "entity", ann.AnnotationType())
}

func TestTermAnnotation_RegisteredInRegistry(t *testing.T) {
	ann, ok := model.NewAnnotation("term")
	assert.True(t, ok)
	assert.NotNil(t, ann)
	assert.Equal(t, "term", ann.AnnotationType())
}

func TestEntityAnnotation_Source(t *testing.T) {
	ea := &model.EntityAnnotation{
		Text:   "John Smith",
		Type:   model.EntityPerson,
		Source: model.ExtractionSourceNER,
	}
	assert.Equal(t, model.ExtractionSourceNER, ea.Source)
}

func TestExtractionSourceConstants(t *testing.T) {
	assert.Equal(t, model.ExtractionSourceLLM, model.ExtractionSource("llm"))
	assert.Equal(t, model.ExtractionSourceNER, model.ExtractionSource("ner"))
	assert.Equal(t, model.ExtractionSourceManual, model.ExtractionSource("manual"))
}

func TestCandidateStatusConstants(t *testing.T) {
	assert.Equal(t, model.CandidateStatusPending, model.CandidateStatus("pending"))
	assert.Equal(t, model.CandidateStatusApproved, model.CandidateStatus("approved"))
	assert.Equal(t, model.CandidateStatusRejected, model.CandidateStatus("rejected"))
}

func TestTermCategoryConstants(t *testing.T) {
	assert.Equal(t, model.TermCategoryBrand, model.TermCategory("brand"))
	assert.Equal(t, model.TermCategoryTechnical, model.TermCategory("technical"))
	assert.Equal(t, model.TermCategoryUI, model.TermCategory("ui"))
	assert.Equal(t, model.TermCategoryLegal, model.TermCategory("legal"))
	assert.Equal(t, model.TermCategoryMarketing, model.TermCategory("marketing"))
	assert.Equal(t, model.TermCategoryGeneral, model.TermCategory("general"))
}

func TestTranslatabilityConstants(t *testing.T) {
	assert.Equal(t, model.TranslatabilityDNT, model.Translatability("dnt"))
	assert.Equal(t, model.TranslatabilityConsistent, model.Translatability("consistent"))
	assert.Equal(t, model.TranslatabilityFree, model.Translatability("free"))
}
