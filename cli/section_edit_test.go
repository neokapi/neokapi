package cli

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/sectionedit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInspectSectionsUsesNativeHeadingIDs(t *testing.T) {
	output := runInspectFixture(t, "page.md", "## Share\n\nFirst.\n\nSecond.\n\n## Keep\n\nKeep.\n", "--sections")
	var doc sectionedit.Document
	require.NoError(t, json.Unmarshal([]byte(output), &doc))
	require.Len(t, doc.Sections, 2)
	assert.Equal(t, "markdown", doc.ContentFormat)
	assert.Len(t, doc.Snapshot, 64)
	assert.Equal(t, doc.Sections[0].Range.Heading.ID, doc.Sections[0].ID)
	assert.Len(t, doc.Sections[0].Range.Body, 2)
}
