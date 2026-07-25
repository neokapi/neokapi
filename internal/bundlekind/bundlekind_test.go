package bundlekind_test

import (
	"testing"

	"github.com/neokapi/neokapi/internal/bundlekind"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnexpected_RetiredKindExplainsTheRename(t *testing.T) {
	for _, tc := range []struct {
		name, pkg, got, want, regen, replacement string
	}{
		{"kbf", "kbf", "kapi-localization-format", "kapi-bundle", "kapi extract", "kapi-bundle"},
		{"kmb", "kmb", "kapi-tm-format", "kapi-memory", "kapi memory export", "kapi-memory"},
		{"ktb", "ktb", "kapi-termbase-format", "kapi-terms", "kapi terms export", "kapi-terms"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := bundlekind.Unexpected(tc.pkg, tc.got, tc.want, tc.regen)
			require.Error(t, err)
			msg := err.Error()
			// A stale file must read as "regenerate me", so the message names the
			// old value, the new value and the command that rewrites the file.
			assert.Contains(t, msg, tc.got)
			assert.Contains(t, msg, tc.replacement)
			assert.Contains(t, msg, "renamed")
			assert.Contains(t, msg, tc.regen)
			assert.Contains(t, msg, "earlier release")
		})
	}
}

func TestUnexpected_UnknownKindStaysABareMismatch(t *testing.T) {
	err := bundlekind.Unexpected("kbf", "something-else", "kapi-bundle", "kapi extract")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unexpected kind "something-else"`)
	assert.Contains(t, err.Error(), `want "kapi-bundle"`)
	assert.NotContains(t, err.Error(), "renamed")
}

func TestUnexpected_MissingKind(t *testing.T) {
	err := bundlekind.Unexpected("ktb", "", "kapi-terms", "kapi terms export")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing kind")
}
