package xliff_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
)

const segmentedFixture = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="two.txt" source-language="en" target-language="fr" datatype="plaintext">
    <body>
      <trans-unit id="1">
        <source>First. Second.</source>
        <seg-source><mrk mtype="seg" mid="s1">First.</mrk> <mrk mtype="seg" mid="s2">Second.</mrk></seg-source>
        <target><mrk mtype="seg" mid="s1">Premier.</mrk> <mrk mtype="seg" mid="s2">Deuxieme.</mrk></target>
      </trans-unit>
    </body>
  </file>
</xliff>`

func TestWriter_PartialTargetRewriteKeepsSegments(t *testing.T) {
	tl := tools.NewSearchReplaceTool(&tools.SearchReplaceConfig{
		Pairs:        []tools.ReplacePair{{Search: "Deuxieme", Replace: "Second"}},
		Target:       true,
		ReplaceAll:   true,
		TargetLocale: "fr",
	})
	got := roundtripThroughTool(t, segmentedFixture, "fr", tl)
	t.Log(got)
	assert.Contains(t, got, `<mrk mtype="seg" mid="s1">Premier.</mrk> <mrk mtype="seg" mid="s2">Second.</mrk>`)
}
