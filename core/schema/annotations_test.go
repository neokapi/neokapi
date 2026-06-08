package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocaleCardinality_Values(t *testing.T) {
	// Verify the typed constants are distinct and non-empty.
	assert.NotEqual(t, Monolingual, Bilingual)
	assert.NotEqual(t, Bilingual, Multilingual)
	assert.NotEqual(t, Monolingual, Multilingual)
	assert.NotEmpty(t, string(Monolingual))
	assert.NotEmpty(t, string(Bilingual))
	assert.NotEmpty(t, string(Multilingual))
}
