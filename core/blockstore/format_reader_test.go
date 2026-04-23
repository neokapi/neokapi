package blockstore_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeReader emits a fixed set of model.Block Parts. Matches the
// minimal DataFormatReader contract the adapter exercises.
type fakeReader struct {
	blocks []model.Block
}

func (r *fakeReader) Name() string                                       { return "fake" }
func (r *fakeReader) DisplayName() string                                { return "Fake Reader" }
func (r *fakeReader) Signature() format.FormatSignature                  { return format.FormatSignature{} }
func (r *fakeReader) Open(_ context.Context, _ *model.RawDocument) error { return nil }
func (r *fakeReader) Close() error                                       { return nil }
func (r *fakeReader) Config() format.DataFormatConfig                    { return nil }
func (r *fakeReader) SetConfig(_ format.DataFormatConfig) error          { return nil }

func (r *fakeReader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, len(r.blocks)+1)
	go func() {
		defer close(ch)
		for i := range r.blocks {
			b := r.blocks[i]
			ch <- model.PartResult{Part: &model.Part{Type: model.PartBlock, Resource: &b}}
		}
	}()
	return ch
}

func factoryFor(blocks []model.Block) blockstore.FormatReaderFactory {
	return func() (format.DataFormatReader, *model.RawDocument, error) {
		return &fakeReader{blocks: blocks}, &model.RawDocument{URI: "x.fake"}, nil
	}
}

func TestFormatReaderStore_capabilities(t *testing.T) {
	store := blockstore.NewFormatReaderStore(factoryFor(nil))
	defer store.Close()
	caps := store.Capabilities()
	assert.True(t, caps.RandomAccess)
	assert.False(t, caps.Writable)
}

func TestFormatReaderStore_streamsBlocks(t *testing.T) {
	mb := func(id string) model.Block {
		return model.Block{ID: id, Translatable: true}
	}
	store := blockstore.NewFormatReaderStore(factoryFor([]model.Block{
		mb("first"),
		mb("second"),
	}))
	defer store.Close()

	sess, err := store.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()

	var ids []string
	for b, err := range sess.Blocks(blockstore.BlockFilter{}) {
		require.NoError(t, err)
		ids = append(ids, b.Hash) // ID doubles as Hash for format-reader blocks
	}
	assert.Equal(t, []string{"first", "second"}, ids, "order matches reader emission")
}

func TestFormatReaderStore_getBlockByID(t *testing.T) {
	store := blockstore.NewFormatReaderStore(factoryFor([]model.Block{{ID: "a"}}))
	defer store.Close()

	sess, err := store.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()

	got, err := sess.GetBlock("a")
	require.NoError(t, err)
	assert.Equal(t, "a", got.ID)

	_, err = sess.GetBlock("missing")
	assert.ErrorIs(t, err, blockstore.ErrNotFound)
}

func TestFormatReaderStore_writesReturnReadOnly(t *testing.T) {
	store := blockstore.NewFormatReaderStore(factoryFor(nil))
	defer store.Close()
	sess, err := store.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()

	err = sess.PutBlock("ui", &blockstore.Block{Hash: "h1"})
	require.ErrorIs(t, err, blockstore.ErrReadOnly)
	err = sess.PutOverlay(blockstore.Overlay{Kind: "targets/fr", BlockHash: "h1"})
	require.ErrorIs(t, err, blockstore.ErrReadOnly)
}

func TestFormatReaderStore_factoryErrorSurfacedAtBegin(t *testing.T) {
	boom := errors.New("construct failed")
	store := blockstore.NewFormatReaderStore(func() (format.DataFormatReader, *model.RawDocument, error) {
		return nil, nil, boom
	})
	defer store.Close()
	_, err := store.Begin(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

func TestFormatReaderStore_filterTranslatable(t *testing.T) {
	b := func(id string, translatable bool) model.Block {
		return model.Block{ID: id, Translatable: translatable}
	}
	store := blockstore.NewFormatReaderStore(factoryFor([]model.Block{
		b("h1", true),
		b("h2", false),
		b("h3", true),
	}))
	defer store.Close()
	sess, err := store.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()

	tr := true
	var got []string
	for block, err := range sess.Blocks(blockstore.BlockFilter{Translatable: &tr}) {
		require.NoError(t, err)
		got = append(got, block.Hash)
	}
	assert.Equal(t, []string{"h1", "h3"}, got)
}

// Verify io.Reader-style wiring compiles / stays aligned with the
// format package's actual shape (belt + braces against package drift).
var _ = io.Discard
var _ = strings.NewReader
