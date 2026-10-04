package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host/output"
)

// decodeFormats parses the --json listing of kapi formats.
func decodeFormats(t *testing.T, out string) output.FormatsListOutput {
	t.Helper()
	var list output.FormatsListOutput
	require.NoError(t, json.Unmarshal([]byte(out), &list), out)
	return list
}

func TestFormats_BareCommandListsFormats(t *testing.T) {
	out, err := runFormats(t, "--json")
	require.NoError(t, err, out)
	list := decodeFormats(t, out)
	require.NotEmpty(t, list.Formats)
	assert.Equal(t, len(list.Formats), list.Total)
	names := map[string]bool{}
	for _, f := range list.Formats {
		names[f.Name] = true
	}
	for _, want := range []string{"html", "json", "po"} {
		assert.True(t, names[want], "%s is listed", want)
	}

	text, err := runFormats(t)
	require.NoError(t, err, text)
	assert.Contains(t, text, "json")
	assert.Contains(t, text, "html")
}

// kapi formats list is the spelling the other command families use; it prints
// exactly what the bare command prints, in both forms.
func TestFormats_ListPrintsWhatTheBareCommandPrints(t *testing.T) {
	bare, err := runFormats(t, "--json")
	require.NoError(t, err, bare)
	listed, err := runFormats(t, "list", "--json")
	require.NoError(t, err, listed)
	assert.JSONEq(t, bare, listed)

	bareText, err := runFormats(t)
	require.NoError(t, err, bareText)
	listedText, err := runFormats(t, "list")
	require.NoError(t, err, listedText)
	assert.Equal(t, bareText, listedText)
}

func TestFormats_ListFiltersByExtension(t *testing.T) {
	out, err := runFormats(t, "list", "--json", "--ext", ".json")
	require.NoError(t, err, out)
	list := decodeFormats(t, out)
	require.NotEmpty(t, list.Formats)
	assert.Equal(t, len(list.Formats), list.Total)
	var names []string
	for _, f := range list.Formats {
		names = append(names, f.Name)
		hasJSON := false
		for _, ext := range f.Extensions {
			if strings.EqualFold(ext, ".json") {
				hasJSON = true
			}
		}
		assert.True(t, hasJSON, "%s reads .json files (%v)", f.Name, f.Extensions)
	}
	assert.Contains(t, names, "json")
	assert.NotContains(t, names, "html")

	bare, err := runFormats(t, "--json", "--ext", ".json")
	require.NoError(t, err, bare)
	assert.JSONEq(t, bare, out, "the bare command filters the same way")

	none, err := runFormats(t, "list", "--json", "--ext", ".no-such-extension")
	require.NoError(t, err, none)
	assert.Empty(t, decodeFormats(t, none).Formats)
}

// A format named without --ops is refused with a pointer to kapi formats
// info, under either spelling of the listing.
func TestFormats_RefusesAStrayArgument(t *testing.T) {
	for _, args := range [][]string{{"html"}, {"list", "html"}} {
		_, err := runFormats(t, args...)
		require.Error(t, err, "kapi formats %s", strings.Join(args, " "))
		assert.Equal(t, ExitUsage, ExitCode(nil, err))
		assert.Contains(t, err.Error(), "kapi formats takes a format name only with --ops")
		assert.Contains(t, err.Error(), "kapi formats info html")
	}
}
