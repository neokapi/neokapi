package jsx_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
)

// emptyLocaleBundle is a bundle in schema 1 whose block carries targets in the
// shape that schema keyed them by: a map from locale to runs beside a map from
// locale to origin. A target under the empty locale is a translation filed
// under no language; kbf.ValidateBlock accepts it.
func emptyLocaleBundle(targets, origins string) string {
	return `{
  "schemaVersion": "1.0",
  "kind": "kapi-bundle",
  "generator": {"id": "test", "version": "0"},
  "project": {"id": "p", "sourceLocale": "en"},
  "documents": [{
    "id": "d1",
    "documentType": "jsx",
    "path": "app/Greeting.tsx",
    "blocks": [{
      "id": "d1:b1",
      "hash": "h1",
      "translatable": true,
      "type": "jsx:element",
      "source": [{"text": "Sign in"}],
      "targets": ` + targets + `,
      "targetOrigins": ` + origins + `,
      "placeholders": [],
      "properties": {}
    }]
  }]
}`
}

// A bundle's target under the empty locale reads as the block's translation
// filed under no language, and is written back as the unlabelled edition with
// its origin, beside the editions the edition accessors reach.
func TestKBFRoundTripKeepsATargetUnderTheEmptyLocale(t *testing.T) {
	cases := []struct {
		name           string
		targets        string
		origins        string
		wantEditions   map[string]string
		wantUnlabelled string
		wantOrigin     string
	}{
		{
			name:           "with an origin",
			targets:        `{"nb": [{"text": "Logg inn"}], "": [{"text": "EMPTYLOC"}]}`,
			origins:        `{"nb": {"kind": "ai"}, "": {"kind": "human"}}`,
			wantEditions:   map[string]string{"nb": "Logg inn"},
			wantUnlabelled: "EMPTYLOC",
			wantOrigin:     "human",
		},
		{
			name:           "without an origin",
			targets:        `{"": [{"text": "EMPTYLOC"}]}`,
			origins:        `{}`,
			wantEditions:   map[string]string{},
			wantUnlabelled: "EMPTYLOC",
		},
		{
			name:         "an origin with no runs is not written",
			targets:      `{"nb": [{"text": "Logg inn"}], "": []}`,
			origins:      `{"": {"kind": "human"}}`,
			wantEditions: map[string]string{"nb": "Logg inn"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocks := readBlocks(t, emptyLocaleBundle(tc.targets, tc.origins))
			require.Len(t, blocks, 1)
			assert.Equal(t, "Sign in", blocks[0].SourceText())
			assert.Equal(t, tc.wantUnlabelled, blocks[0].TargetText(""))

			body := writeBundle(t, blocks[0])
			var file kbf.File
			require.NoError(t, json.Unmarshal([]byte(body), &file))
			require.Len(t, file.Documents, 1)
			require.Len(t, file.Documents[0].Blocks, 1)
			out := file.Documents[0].Blocks[0]

			assert.Equal(t, "Sign in", model.RunsText(out.SourceRuns()))
			got := map[string]string{}
			for _, key := range out.TargetKeys() {
				got[key] = model.RunsText(out.Editions[key].Runs)
			}
			assert.Equal(t, tc.wantEditions, got)
			if tc.wantUnlabelled == "" {
				assert.Nil(t, out.Unlabelled)
				return
			}
			require.NotNil(t, out.Unlabelled)
			assert.Equal(t, tc.wantUnlabelled, model.RunsText(out.Unlabelled.Runs))
			assert.Equal(t, tc.wantOrigin, out.Unlabelled.Origin.Kind)

			back := readBlocks(t, body)
			require.Len(t, back, 1)
			assert.Equal(t, tc.wantUnlabelled, back[0].TargetText(""))
			assert.Equal(t, "Sign in", back[0].SourceText(), "the unlabelled edition never reaches the source")
		})
	}
}

// A block that reaches the writer with no bundle annotation, read from another
// format, keeps a translation filed under no language too.
func TestKBFWriterKeepsTheUnlabelledEditionOfABlockFromAnotherFormat(t *testing.T) {
	b := model.NewBlock("s1", "Sign in")
	b.SetTargetEdition(model.EditionKey{}, model.Edition{
		Runs:   []model.Run{model.TextR("EMPTYLOC")},
		Origin: model.Origin{Kind: model.OriginHuman},
	})

	var file kbf.File
	require.NoError(t, json.Unmarshal([]byte(writeBundle(t, b)), &file))
	out := file.Documents[0].Blocks[0]
	require.NotNil(t, out.Unlabelled)
	assert.Equal(t, "EMPTYLOC", model.RunsText(out.Unlabelled.Runs))
	assert.Equal(t, model.OriginHuman, out.Unlabelled.Origin.Kind)
	assert.Empty(t, out.TargetKeys())
}
