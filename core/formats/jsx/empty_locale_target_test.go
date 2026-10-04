package jsx_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
)

// emptyLocaleBundle is a bundle whose block carries, beside its nb target, a
// target under the empty locale. kbf.ValidateBlock accepts it, and the reader
// files it on the block like any other target.
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

// A bundle's target under the empty locale is written back as it was read,
// with its origin, beside the targets the edition accessors reach.
func TestKBFRoundTripKeepsATargetUnderTheEmptyLocale(t *testing.T) {
	cases := []struct {
		name        string
		targets     string
		origins     string
		wantTargets map[kbf.LocaleID]string
		wantOrigins []kbf.LocaleID
	}{
		{
			name:        "with an origin",
			targets:     `{"nb": [{"text": "Logg inn"}], "": [{"text": "EMPTYLOC"}]}`,
			origins:     `{"nb": {"kind": "ai"}, "": {"kind": "human"}}`,
			wantTargets: map[kbf.LocaleID]string{"nb": "Logg inn", "": "EMPTYLOC"},
			wantOrigins: []kbf.LocaleID{"", "nb"},
		},
		{
			name:        "without an origin",
			targets:     `{"": [{"text": "EMPTYLOC"}]}`,
			origins:     `{}`,
			wantTargets: map[kbf.LocaleID]string{"": "EMPTYLOC"},
		},
		{
			name:        "an origin with no runs is not written",
			targets:     `{"nb": [{"text": "Logg inn"}], "": []}`,
			origins:     `{"": {"kind": "human"}}`,
			wantTargets: map[kbf.LocaleID]string{"nb": "Logg inn"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocks := readBlocks(t, emptyLocaleBundle(tc.targets, tc.origins))
			require.Len(t, blocks, 1)
			assert.Equal(t, "Sign in", blocks[0].SourceText())

			var file kbf.File
			require.NoError(t, json.Unmarshal([]byte(writeBundle(t, blocks[0])), &file))
			require.Len(t, file.Documents, 1)
			require.Len(t, file.Documents[0].Blocks, 1)
			out := file.Documents[0].Blocks[0]

			assert.Equal(t, "Sign in", model.RunsText(out.Source))
			got := map[kbf.LocaleID]string{}
			for loc, runs := range out.Targets {
				got[loc] = model.RunsText(runs)
			}
			assert.Equal(t, tc.wantTargets, got)
			var origins []kbf.LocaleID
			for loc := range out.TargetOrigins {
				origins = append(origins, loc)
			}
			assert.ElementsMatch(t, tc.wantOrigins, origins)
			if tc.wantOrigins != nil {
				assert.Equal(t, "human", out.TargetOrigins[""].Kind)
			}
		})
	}
}
