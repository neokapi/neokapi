package structdoc

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/structrec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yamlv3 "gopkg.in/yaml.v3"
)

// blockPart wraps a block with role/level into a PartBlock.
func blockPart(id, text, role string, level int) *model.Part {
	b := model.NewBlock(id, text)
	if role != "" || level != 0 {
		b.SetSemanticRole(role, level)
	}
	return &model.Part{Type: model.PartBlock, Resource: b}
}

func feed(parts ...*model.Part) <-chan *model.Part {
	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	return ch
}

func runWriter(t *testing.T, w *Writer, parts ...*model.Part) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&buf))
	require.NoError(t, w.Write(context.Background(), feed(parts...)))
	return buf.String()
}

// A structural document (headings + paragraphs, no catalog keys) serializes as a
// JSON array of records — never the empty `{}` the catalog writer produced.
func TestJSONWriter_StructuralArray(t *testing.T) {
	out := runWriter(t, NewJSONWriter(),
		blockPart("b1", "Q3 Report", model.RoleHeading, 1),
		blockPart("b2", "Revenue grew.", model.RoleParagraph, 0),
	)
	require.NotEqual(t, "{}", bytes.TrimSpace([]byte(out)))

	var recs []structrec.Record
	require.NoError(t, json.Unmarshal([]byte(out), &recs))
	require.Len(t, recs, 2)

	assert.Equal(t, 1, recs[0].Number)
	assert.Equal(t, "b1", recs[0].ID)
	assert.Equal(t, "Q3 Report", recs[0].Text)
	assert.Equal(t, model.RoleHeading, recs[0].Role)
	assert.Equal(t, 1, recs[0].Level)
	assert.Equal(t, model.ComputeContentHash("Q3 Report"), recs[0].ContentHash)

	assert.Equal(t, 2, recs[1].Number)
	assert.Equal(t, model.RoleParagraph, recs[1].Role)
}

// Empty input renders as an empty array, not `{}` or `"": ...`.
func TestJSONWriter_Empty(t *testing.T) {
	out := runWriter(t, NewJSONWriter())
	assert.Equal(t, "[]", string(bytes.TrimSpace([]byte(out))))
}

func TestYAMLWriter_StructuralSequence(t *testing.T) {
	out := runWriter(t, NewYAMLWriter(),
		blockPart("b1", "Title", model.RoleHeading, 2),
		blockPart("b2", "Body text.", model.RoleParagraph, 0),
	)
	var recs []structrec.Record
	require.NoError(t, yamlv3.Unmarshal([]byte(out), &recs))
	require.Len(t, recs, 2)
	assert.Equal(t, "Title", recs[0].Text)
	assert.Equal(t, model.RoleHeading, recs[0].Role)
	assert.Equal(t, 2, recs[0].Level)
}

func TestYAMLWriter_Empty(t *testing.T) {
	out := runWriter(t, NewYAMLWriter())
	assert.Equal(t, "[]", string(bytes.TrimSpace([]byte(out))))
}

// Blocks with no text are skipped so they never appear as empty records.
func TestJSONWriter_SkipsEmptyBlocks(t *testing.T) {
	out := runWriter(t, NewJSONWriter(),
		blockPart("b1", "Kept", model.RoleParagraph, 0),
		blockPart("b2", "", model.RoleParagraph, 0),
	)
	var recs []structrec.Record
	require.NoError(t, json.Unmarshal([]byte(out), &recs))
	require.Len(t, recs, 1)
	assert.Equal(t, "Kept", recs[0].Text)
}
