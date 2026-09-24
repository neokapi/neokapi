package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// utilizeFindings are the vocabulary findings a report raises for `utilize`.
func utilizeFindings(report check.Report) []check.Diagnostic {
	var out []check.Diagnostic
	for _, d := range report.Findings {
		if d.Rule == "voice.vocabulary" && strings.Contains(d.Message, `"utilize"`) {
			out = append(out, d)
		}
	}
	return out
}

// scopedTextCheckFixture forbids `utilize` in the child channel's vocabulary
// and nowhere else, so the same sentence is a finding at one point and not at
// the other, as a draft and as a saved file.
func TestCheckChannelVocabularyAppliesOnlyInItsChannel(t *testing.T) {
	app, root := scopedTextCheckFixture(t)
	t.Chdir(t.TempDir())
	const text = "Utilize the new route."

	for channel, want := range map[string]int{"child": 1, "adult": 0} {
		t.Run(channel, func(t *testing.T) {
			_, draft, err := app.checkTextMCP(t.Context(), checkTextInput{
				Text: text, ContextPath: channel + "/draft.json",
			})
			require.NoError(t, err)
			found := utilizeFindings(draft)
			require.Len(t, found, want, "the child channel's forbidden term applies in that channel only")
			for _, d := range found {
				assert.True(t, d.Fails, "the rule keeps the severity the channel gave it")
			}

			file := filepath.Join(root, channel, "page.json")
			require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o700))
			body, err := json.Marshal(map[string]string{"body": text})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(file, body, 0o600))
			_, saved, err := app.checkFileMCP(t.Context(), checkFileInput{File: file})
			require.NoError(t, err)
			assert.Len(t, utilizeFindings(saved), want, "a saved file resolves the same channel vocabulary")
		})
	}
}
