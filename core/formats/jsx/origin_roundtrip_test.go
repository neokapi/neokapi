package jsx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/jsx"
	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A target's provenance has to survive a write and a read, or an answer
// re-seeded from a bundle arrives with no governing context recorded — which
// reads identically to an answer produced under no governance at all, and
// cannot be judged (#2278).

// originFixture is a bundle whose one target carries a full provenance record.
const originFixture = `{
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
      "targets": {"nb": [{"text": "Logg inn"}]},
      "targetOrigins": {"nb": {
        "kind": "ai",
        "engine": "claude",
        "tool": "translate",
        "profile": "northsea",
        "profile_version": "3",
        "context_fingerprint": "cfp-abc123"
      }},
      "placeholders": [],
      "properties": {}
    }]
  }]
}`

// readBlocks runs the fixture through the reader and returns its blocks.
func readBlocks(t *testing.T, body string) []*model.Block {
	t.Helper()
	r := jsx.NewReader()
	doc := &model.RawDocument{Reader: io.NopCloser(strings.NewReader(body)), URI: "in.kbf.json"}
	require.NoError(t, r.Open(context.Background(), doc))
	t.Cleanup(func() { _ = r.Close() })

	var out []*model.Block
	for res := range r.Read(context.Background()) {
		require.NoError(t, res.Error)
		if res.Part == nil {
			continue
		}
		if b, ok := res.Part.Resource.(*model.Block); ok {
			out = append(out, b)
		}
	}
	return out
}

func TestKBFReaderRestoresTargetOrigin(t *testing.T) {
	blocks := readBlocks(t, originFixture)
	require.Len(t, blocks, 1)

	tgt := blocks[0].Target("nb")
	require.NotNil(t, tgt, "the bundle declares an nb target")
	assert.Equal(t, "Logg inn", blocks[0].TargetText("nb"))

	// The whole record, not a chosen field: a reader that restored only the
	// fingerprint would still lose which profile produced the answer.
	assert.Equal(t, "ai", tgt.Origin.Kind)
	assert.Equal(t, "claude", tgt.Origin.Engine)
	assert.Equal(t, "translate", tgt.Origin.Tool)
	assert.Equal(t, "northsea", tgt.Origin.Profile)
	assert.Equal(t, "3", tgt.Origin.ProfileVersion)
	assert.Equal(t, "cfp-abc123", tgt.Origin.ContextFingerprint)
}

func TestKBFTargetOriginSurvivesARoundTrip(t *testing.T) {
	blocks := readBlocks(t, originFixture)
	require.Len(t, blocks, 1)

	buf := writeBundle(t, blocks[0])

	var file kbf.File
	require.NoError(t, json.Unmarshal([]byte(buf), &file))
	require.Len(t, file.Documents, 1)
	require.Len(t, file.Documents[0].Blocks, 1)

	origin, ok := file.Documents[0].Blocks[0].TargetOrigins["nb"]
	require.True(t, ok, "the written bundle records how its target was produced")
	assert.Equal(t, "cfp-abc123", origin.ContextFingerprint)
	assert.Equal(t, "northsea", origin.Profile)
	assert.Equal(t, "ai", origin.Kind)

	// Read the written bytes back: the round trip is what the absorber depends
	// on, so it is asserted end to end rather than one leg at a time.
	again := readBlocks(t, buf)
	require.Len(t, again, 1)
	back := again[0].Target("nb")
	require.NotNil(t, back)
	assert.Equal(t, "cfp-abc123", back.Origin.ContextFingerprint)
	assert.Equal(t, "Logg inn", again[0].TargetText("nb"))
}

func TestKBFWritesNoOriginWhenNoneWasStamped(t *testing.T) {
	blocks := readBlocks(t, originFixture)
	require.Len(t, blocks, 1)
	// A producer that stamped nothing leaves the record empty; an empty record
	// and no record are the same fact, and writing one would grow every bundle
	// for nothing.
	blocks[0].StampTargetProvenance("nb", "", model.Origin{})

	buf := writeBundle(t, blocks[0])

	assert.NotContains(t, buf, "targetOrigins")
	// The translation itself is untouched by the absence of provenance.
	assert.Contains(t, buf, "Logg inn")
}

func TestKBFReadsABundleWithoutOrigins(t *testing.T) {
	// Every bundle written before the field existed is still valid, and reads
	// as an answer with no governance recorded.
	body := strings.Replace(originFixture, `"targetOrigins": {"nb": {`, `"unusedOrigins": {"nb": {`, 1)
	blocks := readBlocks(t, body)
	require.Len(t, blocks, 1)

	tgt := blocks[0].Target("nb")
	require.NotNil(t, tgt)
	assert.Equal(t, "Logg inn", blocks[0].TargetText("nb"))
	assert.Equal(t, model.Origin{}, tgt.Origin)
}

// writeBundle runs one block through the writer and returns the emitted JSON.
func writeBundle(t *testing.T, b *model.Block) string {
	t.Helper()
	var sink bytes.Buffer
	w := jsx.NewWriter()
	require.NoError(t, w.SetOutputWriter(&sink))

	ch := make(chan *model.Part, 2)
	ch <- &model.Part{Type: model.PartBlock, Resource: b}
	close(ch)
	require.NoError(t, w.Write(context.Background(), ch))
	require.NoError(t, w.Close())
	return sink.String()
}

var _ format.DataFormatWriter = (*jsx.Writer)(nil)
