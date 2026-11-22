package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A person's followed languages are one list per project: none saved reads
// as unsaved, saving again replaces it, an empty list is a saved choice, and
// another person or project holds its own.
func TestFollowedLanguages_OneListPerPersonAndProject(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	other := createTestProject(t, s)

	got, saved, err := s.FollowedLanguages(ctx, "u-ada", p.ID)
	require.NoError(t, err)
	assert.False(t, saved, "nothing saved yet")
	assert.Empty(t, got)

	require.NoError(t, s.PutFollowedLanguages(ctx, "u-ada", p.ID, []string{"nb", "sv"}))
	require.NoError(t, s.PutFollowedLanguages(ctx, "u-ada", p.ID, []string{"nb"}))
	require.NoError(t, s.PutFollowedLanguages(ctx, "u-ada", other.ID, nil))
	require.NoError(t, s.PutFollowedLanguages(ctx, "u-ben", p.ID, []string{"de"}))

	got, saved, err = s.FollowedLanguages(ctx, "u-ada", p.ID)
	require.NoError(t, err)
	assert.True(t, saved)
	assert.Equal(t, []string{"nb"}, got, "saving again replaces the list")

	got, saved, err = s.FollowedLanguages(ctx, "u-ada", other.ID)
	require.NoError(t, err)
	assert.True(t, saved, "an empty list is a saved choice")
	assert.Empty(t, got)

	all, err := s.ListFollowedLanguages(ctx, "u-ada")
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{p.ID: {"nb"}, other.ID: {}}, all)

	require.Error(t, s.PutFollowedLanguages(ctx, "", p.ID, nil), "a list names its person")
}
