package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
)

// runFormats runs kapi formats with args and returns what it printed.
func runFormats(t *testing.T, args ...string) (string, error) {
	t.Helper()
	a := &App{}
	a.InitRegistries()
	cmd := NewFormatsCmd(a)
	// Persistent, as the root's --json is, so `formats list --json` reaches it.
	cmd.PersistentFlags().Bool("json", false, "")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// kapi formats --ops says what a format takes of a change set, as
// describe_format does: one format named prints its description.
func TestFormatsOps_DescribesOneFormat(t *testing.T) {
	out, err := runFormats(t, "--ops", "json", "--json")
	require.NoError(t, err, out)
	var d change.Description
	require.NoError(t, json.Unmarshal([]byte(out), &d), out)
	assert.Equal(t, "json", d.Format)
	assert.Equal(t, change.EditionsPerFile, d.Editions)
	assert.NotNil(t, d.Ops.InsertBlock, "a JSON catalog takes insert_block")
	assert.Contains(t, d.Supported(), change.KindReplaceText)

	out, err = runFormats(t, "--ops", "po", "--json")
	require.NoError(t, err, out)
	require.NoError(t, json.Unmarshal([]byte(out), &d), out)
	assert.Equal(t, change.EditionsInFile, d.Editions)
	assert.NotNil(t, d.Ops.RemoveEdition)
}

func TestFormatsOps_ListsEveryEditableFormat(t *testing.T) {
	out, err := runFormats(t, "--ops", "--json")
	require.NoError(t, err, out)
	var ds []change.Description
	require.NoError(t, json.Unmarshal([]byte(out), &ds), out)
	names := map[string]bool{}
	for _, d := range ds {
		names[d.Format] = true
	}
	for _, want := range []string{"html", "markdown", "json", "po"} {
		assert.True(t, names[want], "%s is listed", want)
	}

	text, err := runFormats(t, "--ops")
	require.NoError(t, err)
	assert.Contains(t, text, "FORMAT")
	assert.Contains(t, text, "insert_block")
}

func TestFormatsOps_RefusesAnUnknownFormatAndANameWithoutOps(t *testing.T) {
	_, err := runFormats(t, "--ops", "no-such-format")
	require.Error(t, err)
	assert.Equal(t, ExitUsage, ExitCode(nil, err))
	_, err = runFormats(t, "html")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--ops")
}
