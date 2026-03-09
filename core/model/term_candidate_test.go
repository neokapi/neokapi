package model_test

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/stretchr/testify/assert"
)

func TestTermCandidateAnnotation_AnnotationType(t *testing.T) {
	tc := &model.TermCandidateAnnotation{
		Text:            "Dashboard",
		Definition:      "Main overview screen",
		Category:        model.TermCategoryUI,
		Translatability: model.TranslatabilityConsistent,
		Confidence:      0.92,
		Position:        model.TextRange{Start: 5, End: 14},
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
	assert.Equal(t, model.ExtractionSource("llm"), model.ExtractionSourceLLM)
	assert.Equal(t, model.ExtractionSource("ner"), model.ExtractionSourceNER)
	assert.Equal(t, model.ExtractionSource("manual"), model.ExtractionSourceManual)
}

func TestCandidateStatusConstants(t *testing.T) {
	assert.Equal(t, model.CandidateStatus("pending"), model.CandidateStatusPending)
	assert.Equal(t, model.CandidateStatus("approved"), model.CandidateStatusApproved)
	assert.Equal(t, model.CandidateStatus("rejected"), model.CandidateStatusRejected)
}

func TestTermCategoryConstants(t *testing.T) {
	assert.Equal(t, model.TermCategory("brand"), model.TermCategoryBrand)
	assert.Equal(t, model.TermCategory("technical"), model.TermCategoryTechnical)
	assert.Equal(t, model.TermCategory("ui"), model.TermCategoryUI)
	assert.Equal(t, model.TermCategory("legal"), model.TermCategoryLegal)
	assert.Equal(t, model.TermCategory("marketing"), model.TermCategoryMarketing)
	assert.Equal(t, model.TermCategory("general"), model.TermCategoryGeneral)
}

func TestTranslatabilityConstants(t *testing.T) {
	assert.Equal(t, model.Translatability("dnt"), model.TranslatabilityDNT)
	assert.Equal(t, model.Translatability("consistent"), model.TranslatabilityConsistent)
	assert.Equal(t, model.Translatability("free"), model.TranslatabilityFree)
}
