package html

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// The token reader trims a bare text block's trailing whitespace when the next
// structural event closes its text unit. A part it has sent belongs to the
// consumer, which may read or edit it at once, so the reader sends a text
// block, and every part read after it, only once that trim is settled.
//
// The channel is unbuffered, so the reader cannot read past a part until the
// consumer has taken it: the attribute block of the <img> between the text and
// the <p> holds the reader until the consumer has seen the text block. A
// reader that trimmed a sent block would change it after the consumer saw it.
func TestTokenReaderSendsABlockOnlyOnceItIsSettled(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"inline part between text and block", `<html><body><div>Hello world <img alt="A picture"><p>Para</p></div></body></html>`},
		{"inline element between text and block", `<html><body><div>Hello <b>bold</b> world <img alt="Pic"> <p>Para</p></div></body></html>`},
		{"text at the end of the document", `<div><p>Para</p>Tail text <img alt="Pic"> `},
		{"two text units", `<div><p>A</p>First unit <img alt="One"><p>B</p>Second unit <img alt="Two"><hr></div>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewReader()
			store, err := format.NewSkeletonStore()
			require.NoError(t, err)
			defer store.Close()
			r.SetSkeletonStore(store)
			state := newTokenReaderState(r, store)

			ch := make(chan model.PartResult)
			go func() {
				defer close(ch)
				state.run([]byte(tc.input), t.Context(), ch)
			}()

			type sent struct {
				block *model.Block
				text  string
			}
			var blocks []sent
			for pr := range ch {
				require.NoError(t, pr.Error)
				if b, ok := pr.Part.Resource.(*model.Block); ok {
					blocks = append(blocks, sent{b, model.RenderRunsWithData(b.SourceRuns())})
				}
			}
			require.NotEmpty(t, blocks)
			for _, s := range blocks {
				assert.Equal(t, s.text, model.RenderRunsWithData(s.block.SourceRuns()), "block %s changed after it was sent", s.block.ID)
			}
		})
	}
}

// A consumer that edits each block as it arrives, as a flow does, writes the
// same document as one that edits once the reader has finished, every time.
func TestTokenReaderEditDuringTheReadWritesAsAnEditAfterIt(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("<html><body>")
	for i := range 200 {
		fmt.Fprintf(&sb, "<div><p>Para %d</p>Unit %d text </div>\n", i, i)
	}
	sb.WriteString("</body></html>")
	input := []byte(sb.String())

	want := readEditWrite(t, input, false)
	for range 5 {
		assert.Equal(t, want, readEditWrite(t, input, true))
	}
}

// readEditWrite reads input wired to a skeleton store, upper-cases every
// block's source as it arrives (during) or once the read is done, and writes
// the document back.
func readEditWrite(t *testing.T, input []byte, during bool) string {
	t.Helper()
	upper := func(b *model.Block) {
		runs := slices.Clone(b.SourceRuns())
		for i := range runs {
			if runs[i].Text != nil {
				text := *runs[i].Text
				text.Text = strings.ToUpper(text.Text)
				runs[i].Text = &text
			}
		}
		b.EditSourceRuns(runs)
	}
	r, w := NewReader(), NewWriter()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	r.SetSkeletonStore(store)
	w.SetSkeletonStore(store)
	require.NoError(t, r.Open(t.Context(), &model.RawDocument{URI: "doc.html", SourceLocale: model.LocaleEnglish, Reader: io.NopCloser(bytes.NewReader(input))}))
	var parts []*model.Part
	for res := range r.Read(t.Context()) {
		require.NoError(t, res.Error)
		if b, ok := res.Part.Resource.(*model.Block); ok && during {
			upper(b)
		}
		parts = append(parts, res.Part)
	}
	require.NoError(t, r.Close())
	if !during {
		for _, p := range parts {
			if b, ok := p.Resource.(*model.Block); ok {
				upper(b)
			}
		}
	}
	var out bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&out))
	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	require.NoError(t, w.Write(t.Context(), ch))
	require.NoError(t, w.Close())
	return out.String()
}
