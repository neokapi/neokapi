package messageformat_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/messageformat"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	}
	for _, tc := range cases {
		for _, streaming := range []bool{false, true} {
			for _, mode := range []string{"unchanged", "source", "target"} {
				t.Run(tc.name+"/"+mode+map[bool]string{false: "/buffered", true: "/streaming"}[streaming], func(t *testing.T) {
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
					if mode == "target" {
						writer.SetLocale(model.LocaleID("fr"))
					}
					require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(tc.input, model.LocaleEnglish)))
					var output bytes.Buffer
					require.NoError(t, writer.SetOutputWriter(&output))
					parts := make(chan *model.Part, 16)
					feed := func() {
						defer close(parts)
						defer reader.Close()
						if streaming {
							defer store.CloseWrite()
						}
						for result := range reader.Read(ctx) {
							assert.NoError(t, result.Error)
							if result.Part == nil {
								continue
							}
							if block, ok := result.Part.Resource.(*model.Block); ok && mode != "unchanged" {
								runs := make([]model.Run, len(block.Source))
								copy(runs, block.Source)
								for i, run := range runs {
									if run.Text != nil {
										runs[i].Text = &model.TextRun{Text: strings.ToUpper(run.Text.Text)}
									}
								}
								if mode == "target" {
									block.SetTargetRuns(model.LocaleID("fr"), runs)
								} else {
									block.Source = runs
								}
							}
							parts <- result.Part
						}
					}
					if streaming {
						go feed()
					} else {
						feed()
					}
					require.NoError(t, writer.Write(ctx, parts))
					require.NoError(t, writer.Close())
					want := tc.edited
					if mode == "unchanged" {
						want = tc.input
					}
					assert.Equal(t, want, output.String())
				})
			}
		}
	}
}
