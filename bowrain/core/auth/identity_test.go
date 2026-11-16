package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAPITokenAuthorIdentity pins what a tool token authors as, and that it is
// never something a person could be.
func TestAPITokenAuthorIdentity(t *testing.T) {
	tests := []struct {
		name      string
		token     *APIToken
		want      string
		isMachine bool
		isTool    bool
	}{
		{
			name:  "a personal token authors as its owner",
			token: &APIToken{UserID: "user-123"},
			want:  "user-123",
		},
		{
			name:      "a tool token authors as the tool",
			token:     &APIToken{UserID: "user-123", ToolName: "dogfood-sync"},
			want:      "tool/dogfood-sync",
			isMachine: true,
			isTool:    true,
		},
		{
			name:  "a nil token has no author",
			token: nil,
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.token.AuthorIdentity()
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.isMachine, IsMachineIdentity(got))
			assert.Equal(t, tc.isTool, tc.token.IsTool())
		})
	}

	// The separation-of-duties gate compares an actor to an author with ==, so
	// a tool identity must not collide with the user ID it was minted from.
	tok := &APIToken{UserID: "user-123", ToolName: "user-123"}
	assert.NotEqual(t, tok.UserID, tok.AuthorIdentity(),
		"a tool identity must never equal the identity of the member who issued it")

	name, ok := ToolOfIdentity(tok.AuthorIdentity())
	assert.True(t, ok)
	assert.Equal(t, "user-123", name)
	assert.True(t, IsMachineIdentity("agent/claude"), "an agent acting for a person is a machine identity too")
}

// TestValidateToolName keeps the name half of the identity unambiguous. A name
// carrying "/" could pose as another identity namespace, and a name varying by
// case would let two spellings of one tool look like two actors.
func TestValidateToolName(t *testing.T) {
	valid := []string{"", "ci", "kapi-ci", "github_actions", "a", "a1", "dogfood-sync"}
	for _, name := range valid {
		require.NoError(t, ValidateToolName(name), "expected %q to be accepted", name)
	}

	invalid := []string{
		"Kapi-CI",      // case
		"ai/gpt",       // another identity namespace
		"tool/ci",      // the prefix belongs to the code, not the name
		"-leading",     // must start with a letter or digit
		"trailing-",    // and end with one
		"Dogfood Sync", // a space
		"emoji-🚀",      //
		"ci..ci",       // punctuation outside the allowed set
		"user-123456789012345678901234567890123456789012345678901234567890123", // 65 chars
	}
	for _, name := range invalid {
		err := ValidateToolName(name)
		require.Error(t, err, "expected %q to be rejected", name)
		assert.ErrorIs(t, err, ErrInvalidToolName)
	}
}
