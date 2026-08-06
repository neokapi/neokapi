package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAPITokenAuthorIdentity pins the property the whole machine-identity
// mechanism rests on: what a machine token authors as, and that it is never
// something a person could be.
func TestAPITokenAuthorIdentity(t *testing.T) {
	tests := []struct {
		name      string
		token     *APIToken
		want      string
		isMachine bool
	}{
		{
			name:  "a personal token authors as its owner",
			token: &APIToken{UserID: "user-123"},
			want:  "user-123",
		},
		{
			name:      "a machine token authors as the machine",
			token:     &APIToken{UserID: "user-123", AgentName: "kapi-ci"},
			want:      "agent/kapi-ci",
			isMachine: true,
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
		})
	}

	// The separation-of-duties gate compares an actor to an author with ==, so
	// this is the assertion that makes machine-authored work reviewable: a
	// machine identity cannot collide with the user ID it was minted from.
	tok := &APIToken{UserID: "user-123", AgentName: "user-123"}
	assert.NotEqual(t, tok.UserID, tok.AuthorIdentity(),
		"a machine identity must never equal the identity of the person who minted it")
}

// TestValidateAgentName keeps the name half of the identity unambiguous. A name
// carrying "/" could pose as another identity namespace, and a name varying by
// case would let two spellings of one machine look like two reviewers.
func TestValidateAgentName(t *testing.T) {
	valid := []string{"", "ci", "kapi-ci", "github_actions", "a", "a1", "nightly-sweep-2"}
	for _, name := range valid {
		assert.NoError(t, ValidateAgentName(name), "expected %q to be accepted", name)
	}

	invalid := []string{
		"Kapi-CI",   // case
		"ai/gpt",    // another identity namespace
		"agent/ci",  // the prefix belongs to the code, not the name
		"-leading",  // must start with a letter or digit
		"trailing-", // and end with one
		"has space", //
		"emoji-🚀",   //
		"ci..ci",    // punctuation outside the allowed set
		"user-123456789012345678901234567890123456789012345678901234567890123", // 65 chars
	}
	for _, name := range invalid {
		err := ValidateAgentName(name)
		require.Error(t, err, "expected %q to be rejected", name)
		assert.ErrorIs(t, err, ErrInvalidAgentName)
	}
}
