package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A channel's vocabulary is part of the profile schema, so the strict decode
// that catches typo'd keys accepts it.
func TestVoiceValidate_ChannelVocabularyIsAKnownField(t *testing.T) {
	path := writeTempProfile(t, `name: Service
channels:
  child:
    vocabulary:
      forbidden_terms:
        - term: utilize
          replacement: use
`)
	out, runErr := runVoiceValidate(t, path)

	require.NoError(t, runErr)
	assert.True(t, out.Valid)
	assert.Empty(t, out.Errors, "a channel's vocabulary must not be reported as an unknown field")
}
