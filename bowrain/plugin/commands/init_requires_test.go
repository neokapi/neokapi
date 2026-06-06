package commands

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/plugin/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetServerURL_DeclaresBowrainRequirement verifies that writing a server:
// block also declares `requires: { bowrain: ... }`, so a plain kapi binary
// refuses to load the recipe instead of silently ignoring server:.
func TestSetServerURL_DeclaresBowrainRequirement(t *testing.T) {
	r := &project.Recipe{}
	setServerURL(r, "https://example.test/p/123")

	require.NotNil(t, r.Server)
	assert.Equal(t, "https://example.test/p/123", r.Server.URL)
	require.NotNil(t, r.Requires)
	assert.Equal(t, "*", r.Requires[schema.Group], "server: must declare the bowrain plugin requirement")
}

func TestSetServerURL_EmptyURLIsNoOp(t *testing.T) {
	r := &project.Recipe{}
	setServerURL(r, "")
	assert.Nil(t, r.Server)
	assert.Empty(t, r.Requires[schema.Group], "no server: → no bowrain requirement")
}

func TestRequireBowrain_PreservesExistingRequiresAndIsIdempotent(t *testing.T) {
	r := &project.Recipe{}
	r.Requires = map[string]string{"okapi-bridge": "^1.0"}

	requireBowrain(r)
	requireBowrain(r) // idempotent

	assert.Equal(t, "^1.0", r.Requires["okapi-bridge"], "existing requirements preserved")
	assert.Equal(t, "*", r.Requires[schema.Group])
	assert.Len(t, r.Requires, 2)

	// An explicit pre-set bowrain constraint is not clobbered.
	r2 := &project.Recipe{}
	r2.Requires = map[string]string{schema.Group: "^2.0"}
	requireBowrain(r2)
	assert.Equal(t, "^2.0", r2.Requires[schema.Group])
}
