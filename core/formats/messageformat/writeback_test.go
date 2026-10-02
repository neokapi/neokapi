package messageformat_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/messageformat"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeBack reads input through a skeleton, lets edit change each block, and
// writes the result. With target set, edit is expected to set targets for that
// locale and the writer is pointed at it.
func writeBack(t *testing.T, input string, streaming bool, target model.LocaleID, edit func(*model.Block)) (string, error) {
	t.Helper()
	ctx := t.Context()
	reader := messageformat.NewReader()
	writer := messageformat.NewWriter()
	var store *format.SkeletonStore
	if streaming {
		store = format.NewStreamingSkeletonStore()
	} else {
		var err error
		store, err = format.NewSkeletonStore()
		require.NoError(t, err)
	}
	t.Cleanup(func() { _ = store.Close() })
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)
	if target != "" {
		writer.SetLocale(target)
	}
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	var output bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&output))
	parts := make(chan *model.Part, 16)
	readDone := make(chan error, 1)
	feed := func() {
		var readErr error
		defer func() { readDone <- readErr }()
		defer close(parts)
		defer reader.Close()
		if streaming {
			defer store.CloseWrite()
		}
		for result := range reader.Read(ctx) {
			if result.Error != nil && readErr == nil {
				readErr = result.Error
			}
			if result.Part == nil {
				continue
			}
			if block, ok := result.Part.Resource.(*model.Block); ok && edit != nil {
				edit(block)
			}
			parts <- result.Part
		}
	}
	if streaming {
		go feed()
	} else {
		feed()
	}
	writeErr := writer.Write(ctx, parts)
	require.NoError(t, <-readDone)
	if writeErr != nil {
		return "", writeErr
	}
	require.NoError(t, writer.Close())
	return output.String(), nil
}

// blocksByPath reads input without a skeleton and returns its translatable
// blocks keyed by branch path ("" for a line without a picker).
func blocksByPath(t *testing.T, input string) map[string]*model.Block {
	t.Helper()
	reader := messageformat.NewReader()
	require.NoError(t, reader.Open(t.Context(), testutil.RawDocFromString(input, model.LocaleEnglish)))
	t.Cleanup(func() { _ = reader.Close() })
	blocks := map[string]*model.Block{}
	for _, part := range testutil.CollectParts(t, reader.Read(t.Context())) {
		if block, ok := part.Resource.(*model.Block); ok && block.Translatable {
			blocks[block.Properties["path"]] = block
		}
	}
	return blocks
}

// spell lists runs as kind and payload, so a literal brace in a text run and an
// argument placeholder compare as different content. Adjacent text runs are
// listed as one, since where one text run ends is not content.
func spell(runs []model.Run) []string {
	out := make([]string, 0, len(runs))
	var text strings.Builder
	for i, r := range runs {
		if r.Kind() == model.RunKindText {
			text.WriteString(r.Text.Text)
			if i+1 < len(runs) && runs[i+1].Kind() == model.RunKindText {
				continue
			}
			out = append(out, fmt.Sprintf("text %q", text.String()))
			text.Reset()
			continue
		}
		switch r.Kind() {
		case model.RunKindPh:
			out = append(out, fmt.Sprintf("ph %q", r.Ph.Data))
		default:
			out = append(out, fmt.Sprintf("%s %q", r.Kind(), model.RenderRunsWithData([]model.Run{r})))
		}
	}
	return out
}

func eachMode(t *testing.T, run func(t *testing.T, streaming bool)) {
	for _, streaming := range []bool{false, true} {
		name := map[bool]string{false: "buffered", true: "streaming"}[streaming]
		t.Run(name, func(t *testing.T) { run(t, streaming) })
	}
}

func TestWritebackPreservesArgumentsAndEditsBranches(t *testing.T) {
	cases := []struct {
		name, input, edited string
	}{
		{
			name:   "argument",
			input:  "Hello, {name}!\r\n",
			edited: "HELLO, {name}!\r\n",
		},
		{
			name:   "plural",
			input:  "{count, plural, =0 {empty} one {# item} other {# items}}\n",
			edited: "{count, plural, =0 {EMPTY} one {# ITEM} other {# ITEMS}}\n",
		},
		{
			name:   "single branch",
			input:  "Before {count, plural, other {# items}} after\n",
			edited: "Before {count, plural, other {# ITEMS}} after\n",
		},
		{
			name:   "nested branches",
			input:  "{g, select, one {{n, plural, one {# item} other {# items}}} other {Hello {name}}}\n",
			edited: "{g, select, one {{n, plural, one {# ITEM} other {# ITEMS}}} other {HELLO {name}}}\n",
		},
		{
			name:   "branch edge whitespace",
			input:  "You have{count, plural, one { one item} other { # items}}.\n",
			edited: "You have{count, plural, one { ONE ITEM} other { # ITEMS}}.\n",
		},
		{
			name:   "whitespace on both edges of a branch",
			input:  "{count, plural, one { item } other {\t# items }}\n",
			edited: "{count, plural, one { ITEM } other {\t# ITEMS }}\n",
		},
		{
			name:   "line edge whitespace",
			input:  "  Hello  \n  Hello {name}  \n",
			edited: "  HELLO  \n  HELLO {name}  \n",
		},
	}
	for _, tc := range cases {
		for _, mode := range []string{"unchanged", "source", "target"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				eachMode(t, func(t *testing.T, streaming bool) {
					var target model.LocaleID
					if mode == "target" {
						target = "fr"
					}
					got, err := writeBack(t, tc.input, streaming, target, func(block *model.Block) {
						if mode == "unchanged" {
							return
						}
						runs := make([]model.Run, len(block.Source))
						copy(runs, block.Source)
						for i, run := range runs {
							if run.Text != nil {
								runs[i].Text = &model.TextRun{Text: strings.ToUpper(run.Text.Text)}
							}
						}
						if mode == "target" {
							block.SetTargetRuns(target, runs)
						} else {
							block.Source = runs
						}
					})
					require.NoError(t, err)
					want := tc.edited
					if mode == "unchanged" {
						want = tc.input
					}
					assert.Equal(t, want, got)
				})
			})
		}
	}
}

// TestBranchContentExcludesEdgeWhitespace pins what the slot a block is written
// into covers: the whitespace at the edges of a branch or a line stays in the
// file around the block, whether or not the branch carries a placeholder.
func TestBranchContentExcludesEdgeWhitespace(t *testing.T) {
	blocks := blocksByPath(t, "You have{count, plural, one { one item} other { # items }}.\n")
	require.Contains(t, blocks, "count.one")
	require.Contains(t, blocks, "count.other")
	assert.Equal(t, []string{`text "one item"`}, spell(blocks["count.one"].Source))
	assert.Equal(t, []string{`ph "#"`, `text " items"`}, spell(blocks["count.other"].Source))

	line := blocksByPath(t, "  Hello {name}  \n")
	require.Contains(t, line, "")
	assert.Equal(t, []string{`text "Hello "`, `ph "{name}"`}, spell(line[""].Source))
}
