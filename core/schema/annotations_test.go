package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnnotationRegistry_RegisterAndValidate(t *testing.T) {
	reg := NewAnnotationRegistry()
	reg.Register(AnnotationTypeInfo{
		Type:        AnnotationQAIssues,
		DisplayName: "QA Issues",
		Source:      "built-in",
	})

	require.NoError(t, reg.Validate(AnnotationQAIssues))
	assert.Error(t, reg.Validate("unknown.type"))
}

func TestAnnotationRegistry_Has(t *testing.T) {
	reg := NewAnnotationRegistry()
	assert.False(t, reg.Has(AnnotationQAIssues))

	reg.Register(AnnotationTypeInfo{Type: AnnotationQAIssues, Source: "built-in"})
	assert.True(t, reg.Has(AnnotationQAIssues))
}

func TestAnnotationRegistry_List(t *testing.T) {
	reg := NewAnnotationRegistry()
	reg.Register(AnnotationTypeInfo{Type: AnnotationQAIssues, Source: "built-in"})
	reg.Register(AnnotationTypeInfo{Type: AnnotationWordCount, Source: "built-in"})

	list := reg.List()
	assert.Len(t, list, 2)
}

func TestAnnotationRegistry_RegisterBuiltIns(t *testing.T) {
	reg := NewAnnotationRegistry()
	reg.RegisterBuiltIns()

	// Verify all built-in constants are registered.
	builtins := []AnnotationType{
		AnnotationQAIssues, AnnotationTMMatch, AnnotationAltTranslation,
		AnnotationTerms, AnnotationTermEnforce, AnnotationWordCount,
		AnnotationCharCount, AnnotationSegCount, AnnotationEntityMapping,
		AnnotationComparison, AnnotationScopingReport, AnnotationRepetition,
		AnnotationTranslation,
	}
	for _, at := range builtins {
		require.True(t, reg.Has(at), "missing built-in: %s", at)
	}
}

func TestLocaleCardinality_Values(t *testing.T) {
	// Verify the typed constants are distinct and non-empty.
	assert.NotEqual(t, Monolingual, Bilingual)
	assert.NotEqual(t, Bilingual, Multilingual)
	assert.NotEqual(t, Monolingual, Multilingual)
	assert.NotEmpty(t, string(Monolingual))
	assert.NotEmpty(t, string(Bilingual))
	assert.NotEmpty(t, string(Multilingual))
}
