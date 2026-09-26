package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A channel carries tone and style alone: word rules are terms, so a
// `vocabulary:` under a channel is a field the profile schema does not hold.
func TestVoiceValidate_ChannelVocabularyIsReported(t *testing.T) {
	path := writeTempProfile(t, `name: Service
channels:
  child:
    vocabulary:
      forbidden_terms:
        - term: utilize
          replacement: use
`)
	out, runErr := runVoiceValidate(t, path)

	require.ErrorIs(t, runErr, ErrSilentExit)
	assert.False(t, out.Valid)
	assert.Contains(t, errorFields(out)["vocabulary"], `unknown field "vocabulary" (line 4)`,
		"the channel's word list is named where it sits")
}
