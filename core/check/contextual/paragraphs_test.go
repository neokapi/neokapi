package contextual_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/check/contextual"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParagraphSpansNeedNoReviewRequirements(t *testing.T) {
	text := "Å🙂\r\nline\r\n\r\n同"
	spans, err := contextual.ParagraphSpans(text)
	require.NoError(t, err)
	assert.Equal(t, []contextual.CandidateSpan{
		{ID: "c1", Text: "Å🙂\r\nline", Start: 0, End: 12},
		{ID: "c2", Text: "同", Start: 16, End: 19},
	}, spans)
	for _, invalid := range []string{"", " \r\n\t", string([]byte{0xff})} {
		result, err := contextual.ParagraphSpans(invalid)
		require.Error(t, err)
		assert.Nil(t, result)
	}
}
