package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
)

// A content operation lands through the format's round-trip: only the
// addressed value changes, and the rest of the JSON is byte-identical. Sending
// a block's current text back is unchanged and writes nothing. No provider is
// used.
func TestApplyContentFaithfulRoundTrip(t *testing.T) {
	noProject(t)
	t.Chdir(t.TempDir())
	const src = `{"greeting":"Hello world","note":"Keep me"}`
	require.NoError(t, os.WriteFile("en.json", []byte(src), 0o644))
	app := newToolboxApp(t)
	greeting := recordOf(t, inspectJSONL(t, app, "en.json"), "greeting")

	same := changeSetOf(t, map[string]any{"op": "set_content", "at": greeting.Ref, "if_match": greeting.Rev, "text": "Hello world"})
	res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), same, ApplyOptions{})
	require.NoError(t, err)
	assert.Equal(t, change.OpUnchanged, res.Ops[0].Status)
	require.Len(t, res.Docs, 1)
	assert.False(t, res.Docs[0].Written)
	got, _ := os.ReadFile("en.json")
	assert.Equal(t, src, string(got))

	edit := changeSetOf(t, map[string]any{"op": "set_content", "at": greeting.Ref, "if_match": greeting.Rev, "text": "Hi planet"})
	res, err = applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), edit, ApplyOptions{})
	require.NoError(t, err)
	assert.Equal(t, change.OpApplied, res.Ops[0].Status)
	got, _ = os.ReadFile("en.json")
	assert.Equal(t, `{"greeting":"Hi planet","note":"Keep me"}`, string(got))
}

// A change set that names a document that does not exist is refused whole:
// the document's operations are not_found, every other one is not applied,
// and no file is written.
func TestApplyRefusesASetThatNamesAMissingFile(t *testing.T) {
	noProject(t)
	dir := t.TempDir()
	t.Chdir(dir)
	good := filepath.Join(dir, "en.json")
	require.NoError(t, os.WriteFile(good, []byte(`{"greeting":"Hello world"}`), 0o644))
	app := newToolboxApp(t)
	greeting := recordOf(t, inspectJSONL(t, app, "en.json"), "greeting")

	body := changeSetOf(t,
		map[string]any{"op": "set_content", "at": map[string]any{"doc": "missing.json", "block": "greeting"}, "if_match": "*", "text": "Hi planet"},
		map[string]any{"op": "set_content", "at": greeting.Ref, "if_match": greeting.Rev, "text": "Hi planet"})
	res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), body, ApplyOptions{})
	assert.Equal(t, ExitGate, ExitCode(nil, err))
	assert.Equal(t, change.SetRefused, res.Status)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeNotFound, res.Ops[0].Error.Code)
	assert.Contains(t, res.Ops[0].Error.Message, "missing.json")
	assert.Equal(t, change.OpNotApplied, res.Ops[1].Status)

	got, err := os.ReadFile(good)
	require.NoError(t, err)
	assert.JSONEq(t, `{"greeting":"Hello world"}`, string(got))
}
