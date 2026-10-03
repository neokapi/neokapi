package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
)

// An observation says a term was seen in a file. One whose file holds
// neither the term nor a form it avoids is refused, and so is a form to avoid
// that is a description of the term rather than a form of it.
func TestRecordContextObservation_HoldsTheEvidenceToTheFile(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-evidence")
	observe := func(term string, insteadOf ...string) error {
		_, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
			Actor: agentIn("s1"), Project: recipeOf(root), Term: term, InsteadOf: insteadOf,
			Evidence: []contextop.Evidence{{Path: "config/app.yaml"}},
		})
		return err
	}

	err := observe("Harbor Help", "HarborHelp")
	require.Error(t, err, "config/app.yaml holds no Harbor Help")
	assert.Contains(t, err.Error(), `config/app.yaml holds neither "Harbor Help" nor a form it avoids`)

	err = observe("Quickcast", "Quickcast product name variant")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "holds the term \"Quickcast\" with more words")

	assert.NoError(t, observe("Quickcast", "Quick cast"), "the file holds the form it avoids")
}

func TestDescribesTerm(t *testing.T) {
	assert.True(t, describesTerm("Harbor Help", "Harbor Help product name variant"))
	assert.False(t, describesTerm("Harbor Help", "HarborHelp"))
	assert.False(t, describesTerm("Harbor Help", "the Harbor Help"), "one word more can be a form")
	assert.False(t, describesTerm("sign in", "log in"))
}
