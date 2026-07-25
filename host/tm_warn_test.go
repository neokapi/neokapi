package host

import (
	"bytes"
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWarnSuspectTokenEntries covers both representations a placeholder can
// take: literal markup tokens in the text and real inline-code runs. An entry
// whose variants disagree in either representation is one `recycle` will refuse
// to fill from — silent coverage loss unless the seed author is told at ingest.
func TestWarnSuspectTokenEntries(t *testing.T) {
	text := func(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }
	ph := func(equiv string) model.Run {
		return model.Run{Ph: &model.PlaceholderRun{ID: "1", Equiv: equiv, Data: "{" + equiv + "}"}}
	}

	tests := []struct {
		name     string
		variants map[model.LocaleID][]model.Run
		wantWarn bool
	}{
		{
			name: "symmetric plain text is clean",
			variants: map[model.LocaleID][]model.Run{
				"en": {text("Save")},
				"nb": {text("Lagre")},
			},
		},
		{
			name: "symmetric inline-code runs are clean",
			variants: map[model.LocaleID][]model.Run{
				"en": {ph("count"), text(" documented formats")},
				"nb": {ph("count"), text(" dokumenterte formater")},
			},
		},
		{
			name: "a variant missing the peer's inline code warns",
			variants: map[model.LocaleID][]model.Run{
				"en": {ph("documentedCount"), text(" documented formats")},
				"nb": {text(" dokumenterte formater")},
			},
			wantWarn: true,
		},
		{
			name: "a variant with a literal markup token its peer lacks warns",
			variants: map[model.LocaleID][]model.Run{
				"en": {text("Install")},
				"nb": {text("{=m0} Installer")},
			},
			wantWarn: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tm := memory.NewInMemoryStore()
			require.NoError(t, tm.Add(context.Background(), memory.Entry{ID: "e1", Variants: tc.variants}))
			var buf bytes.Buffer
			WarnSuspectTokenEntries(context.Background(), tm, &buf)
			if tc.wantWarn {
				assert.Contains(t, buf.String(), "e1")
				assert.Contains(t, buf.String(), "placeholder set")
			} else {
				assert.Empty(t, buf.String())
			}
		})
	}
}
