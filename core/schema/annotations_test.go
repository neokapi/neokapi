package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnnotationRegistry_RegisterAndValidate(t *testing.T) {
	reg := NewAnnotationRegistry()
	reg.Register(AnnotationTypeInfo{
		Type:        AnnotationFindings,
		DisplayName: "Check Findings",
		Source:      sourceBuiltIn,
	})

	require.NoError(t, reg.Validate(AnnotationFindings))
	assert.Error(t, reg.Validate("unknown.type"))
}

func TestAnnotationRegistry_Has(t *testing.T) {
	reg := NewAnnotationRegistry()
	assert.False(t, reg.Has(AnnotationFindings))

	reg.Register(AnnotationTypeInfo{Type: AnnotationFindings, Source: sourceBuiltIn})
	assert.True(t, reg.Has(AnnotationFindings))
}

func TestAnnotationRegistry_List(t *testing.T) {
	reg := NewAnnotationRegistry()
	reg.Register(AnnotationTypeInfo{Type: AnnotationFindings, Source: sourceBuiltIn})
	reg.Register(AnnotationTypeInfo{Type: AnnotationWordCount, Source: sourceBuiltIn})

	list := reg.List()
	assert.Len(t, list, 2)
}

func TestAnnotationRegistry_RegisterBuiltIns(t *testing.T) {
	reg := NewAnnotationRegistry()
	reg.RegisterBuiltIns()

	// Verify all built-in constants are registered.
	builtins := []AnnotationType{
		AnnotationFindings, AnnotationTMMatch, AnnotationAltTranslation,
		AnnotationTerms, AnnotationTermEnforce, AnnotationWordCount,
		AnnotationCharCount, AnnotationSegCount, AnnotationEntityMapping,
		AnnotationComparison, AnnotationScopingReport, AnnotationRepetition,
		AnnotationTranslation, AnnotationBrandVoice,
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
