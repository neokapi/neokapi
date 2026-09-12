package format

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// textRenderer renders a ref as the block's source text, and a ref whose block
// never arrived as nothing.
func textRenderer(block *model.Block) ([]byte, error) {
	if block == nil {
		return nil, nil
	}
	return []byte(model.RenderRunsWithData(block.Source)), nil
}

func TestBufferedSkeletonWrite(t *testing.T) {
	tests := []struct {
		name       string
		write      func(store *SkeletonStore)
		blocks     map[string]*model.Block
		renderRef  RefRenderer
		renderLang LangRenderer
		want       string
		wantErr    string
	}{
		{
			name: "text and refs interleave",
			write: func(store *SkeletonStore) {
				store.WriteText([]byte("<p>"))
				store.WriteRef("b1")
				store.WriteText([]byte("</p><p>"))
				store.WriteRef("b2")
				store.WriteText([]byte("</p>"))
			},
			blocks: map[string]*model.Block{
				"b1": model.NewBlock("b1", "one"),
				"b2": model.NewBlock("b2", "two"),
			},
			renderRef: textRenderer,
			want:      "<p>one</p><p>two</p>",
		},
		{
			name: "a ref with no block contributes nothing",
			write: func(store *SkeletonStore) {
				store.WriteText([]byte("<p>"))
				store.WriteRef("missing")
				store.WriteText([]byte("</p>"))
			},
			blocks:    map[string]*model.Block{},
			renderRef: textRenderer,
			want:      "<p></p>",
		},
		{
			name: "a renderer may still emit bytes for a missing block",
			write: func(store *SkeletonStore) {
				store.WriteText([]byte(`"key":`))
				store.WriteRef("missing")
			},
			blocks: map[string]*model.Block{},
			renderRef: func(block *model.Block) ([]byte, error) {
				if block == nil {
					return []byte(`""`), nil
				}
				return []byte(model.RenderRunsWithData(block.Source)), nil
			},
			want: `"key":""`,
		},
		{
			name: "a nil lang renderer emits the stored value",
			write: func(store *SkeletonStore) {
				store.WriteText([]byte(`<html lang="`))
				store.WriteLang("en-US")
				store.WriteText([]byte(`">`))
			},
			renderRef: textRenderer,
			want:      `<html lang="en-US">`,
		},
		{
			name: "a lang renderer retargets the stored value",
			write: func(store *SkeletonStore) {
				store.WriteText([]byte(`<html lang="`))
				store.WriteLang("en-US")
				store.WriteText([]byte(`">`))
			},
			renderRef:  textRenderer,
			renderLang: func(string) ([]byte, error) { return []byte("nb-NO"), nil },
			want:       `<html lang="nb-NO">`,
		},
		{
			name: "entries the walker does not act on contribute nothing",
			write: func(store *SkeletonStore) {
				store.WriteText([]byte("<p>"))
				store.WriteOriginal([]byte("one"), []byte("o n e"))
				store.WriteRef("b1")
				store.WriteTrimmed([]byte("one"), []byte("  "))
				store.WriteText([]byte("</p>"))
			},
			blocks:    map[string]*model.Block{"b1": model.NewBlock("b1", "one")},
			renderRef: textRenderer,
			want:      "<p>one</p>",
		},
		{
			name: "a renderer error aborts the write",
			write: func(store *SkeletonStore) {
				store.WriteText([]byte("<p>"))
				store.WriteRef("b1")
				store.WriteText([]byte("</p>"))
			},
			blocks: map[string]*model.Block{"b1": model.NewBlock("b1", "one")},
			renderRef: func(*model.Block) ([]byte, error) {
				return nil, errors.New("bad pattern")
			},
			wantErr: "bad pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMemorySkeletonStore()
			defer store.Close()
			tt.write(store)
			require.NoError(t, store.Flush())

			var out bytes.Buffer
			err := BufferedSkeletonWrite(store, tt.blocks, &out, tt.renderRef, tt.renderLang)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, out.String())
		})
	}
}

// TestBufferedSkeletonWrite_MatchesStreaming is the contract the two walkers
// exist to keep: one skeleton and one renderer, replayed through the buffered
// path and through the streaming path, write the same bytes. A writer that
// hands its renderer to both therefore cannot produce output that depends on
// which path the pipeline chose.
func TestBufferedSkeletonWrite_MatchesStreaming(t *testing.T) {
	type entry struct {
		text string
		ref  string
	}
	entries := []entry{
		{text: "id: "},
		{ref: "b1"},
		{text: "\nname: "},
		{ref: "b2"},
		{text: "\ngone: "},
		{ref: "b3"}, // no block ever arrives for this one
		{text: "\n"},
	}
	blocks := []*model.Block{
		model.NewBlock("b1", "one"),
		model.NewBlock("b2", "two"),
	}
	byID := map[string]*model.Block{}
	for _, b := range blocks {
		byID[b.ID] = b
	}

	buffered := NewMemorySkeletonStore()
	defer buffered.Close()
	streaming := NewStreamingSkeletonStore()
	defer streaming.Close()
	for _, e := range entries {
		if e.ref != "" {
			buffered.WriteRef(e.ref)
			streaming.WriteRef(e.ref)
			continue
		}
		buffered.WriteText([]byte(e.text))
		streaming.WriteText([]byte(e.text))
	}
	require.NoError(t, buffered.Flush())
	streaming.CloseWrite()

	var bufferedOut bytes.Buffer
	require.NoError(t, BufferedSkeletonWrite(buffered, byID, &bufferedOut, textRenderer, nil))

	parts := make(chan *model.Part, len(blocks))
	for _, b := range blocks {
		parts <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(parts)

	var streamedOut bytes.Buffer
	require.NoError(t, StreamSkeletonWrite(context.Background(), streaming, parts, &streamedOut, textRenderer, nil))

	assert.Equal(t, "id: one\nname: two\ngone: \n", bufferedOut.String())
	assert.Equal(t, bufferedOut.String(), streamedOut.String())
}
