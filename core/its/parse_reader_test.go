package its

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractRulesReaderParity asserts ExtractRulesReader (streaming) returns
// the same rules + external refs as ExtractRules ([]byte) on the same document,
// including rules declared deep in the document and external xlink:href refs —
// the cases that make the whole-document assumption load-bearing.
func TestExtractRulesReaderParity(t *testing.T) {
	docs := []string{
		`<?xml version="1.0"?><doc><p>hi</p></doc>`, // no ITS
		`<doc xmlns:its="http://www.w3.org/2005/11/its" its:version="2.0">
			<its:rules version="2.0">
				<its:translateRule selector="//code" translate="no"/>
			</its:rules>
			<p>text</p><code>x</code>
		</doc>`,
		// Rules declared near the END of the document (last-rule-wins), plus an
		// external link — both must be found by the streaming scan.
		`<doc xmlns:its="http://www.w3.org/2005/11/its">
			<p>lots of content here</p>
			<its:rules version="2.0" xmlns:xlink="http://www.w3.org/1999/xlink" xlink:href="ext.xml">
				<its:translateRule selector="//note" translate="no"/>
			</its:rules>
		</doc>`,
	}
	for _, doc := range docs {
		wantRules, wantExt, wantErr := ExtractRules([]byte(doc))
		gotRules, gotExt, gotErr := ExtractRulesReader(strings.NewReader(doc))
		if wantErr != nil {
			require.Error(t, gotErr)
			continue
		}
		require.NoError(t, gotErr)
		assert.Equal(t, wantRules, gotRules, "rules must match ExtractRules")
		assert.Equal(t, wantExt, gotExt, "external refs must match ExtractRules")
	}
}
