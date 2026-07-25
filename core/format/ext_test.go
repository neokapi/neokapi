package format_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtPrefersCompoundSuffix is the contract that makes ".kbf.json" possible:
// the marker segment has to win over the serialization suffix, or every kapi
// bundle resolves to the plain JSON reader.
func TestExtPrefersCompoundSuffix(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"i18n/en-US.kbf.json", ".kbf.json"},
		{"i18n/en-US.KBF.JSON", ".kbf.json"},
		{"app.overlays.json", ".overlays.json"},
		{"app.overlays.jsonl", ".overlays.jsonl"},
		{"l10n/tm/cli-nb.memory.json", ".memory.json"},
		{"l10n/terms.json", ".json"},
		{"seeds/glossary.terms.json", ".terms.json"},

		// Bare suffixes keep resolving to themselves — the compound rule must
		// not capture unrelated files.
		{"messages.json", ".json"},
		{"events.jsonl", ".jsonl"},
		{"doc.xliff", ".xliff"},
		{"archive.kpz", ".kpz"},
		{"README", ""},

		// The registry's extension-only lookup is handed a bare extension
		// rather than a path, so a suffix on its own must resolve to itself.
		{".kbf.json", ".kbf.json"},
		{".memory.json", ".memory.json"},
		{".json", ".json"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, format.Ext(tt.path))
		})
	}
}

// TestExtDivergesFromFilepathExt pins the reason this helper exists at all: on
// every compound suffix the standard library gives the wrong answer.
func TestExtDivergesFromFilepathExt(t *testing.T) {
	for _, path := range []string{
		"en.kbf.json", "a.overlays.json", "a.overlays.jsonl",
		"a.memory.json", "a.terms.json",
	} {
		assert.NotEqual(t, filepath.Ext(path), format.Ext(path),
			"%s: filepath.Ext must be insufficient, otherwise format.Ext is dead weight", path)
	}
}

// TestCompoundSuffixesAreMutuallyExclusive guards the failure mode of a
// suffix table: one entry swallowing another's files.
func TestCompoundSuffixesAreMutuallyExclusive(t *testing.T) {
	exts := []string{
		format.KBFExt, format.OverlaySetExt, format.AnnotationExt,
		format.MemoryBundleExt, format.TermsBundleExt,
	}
	for _, own := range exts {
		path := "sample" + own
		require.True(t, format.HasExt(path, own), "%s must match its own suffix", path)
		for _, other := range exts {
			if other == own {
				continue
			}
			assert.False(t, format.HasExt(path, other),
				"%s must not be read as %s", path, other)
		}
		assert.False(t, format.HasExt(path, ".json"), "%s must not be read as plain .json", path)
	}
}

func TestTrimExtAndStem(t *testing.T) {
	tests := []struct {
		path     string
		wantTrim string
		wantStem string
	}{
		{"i18n/en-US.kbf.json", "i18n/en-US", "en-US"},
		{"l10n/tm/cli-nb.memory.json", "l10n/tm/cli-nb", "cli-nb"},
		{"docs/page.md", "docs/page", "page"},
		{"noext", "noext", "noext"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.wantTrim, format.TrimExt(tt.path))
			assert.Equal(t, tt.wantStem, format.Stem(tt.path))
		})
	}
}

// TestStemDropsWholeCompoundSuffix is the output-naming regression this change
// could easily have shipped: deriving a name with filepath.Ext leaves ".kbf"
// glued to the stem, so `kapi run` over en-US.kbf.json would emit en-US.kbf.*.
func TestStemDropsWholeCompoundSuffix(t *testing.T) {
	assert.Equal(t, "en-US", format.Stem("i18n/en-US.kbf.json"))
	assert.NotContains(t, format.Stem("i18n/en-US.kbf.json"), ".kbf")
}

// TestRetiredExtHintNamesReplacementAndCommand is the self-explaining-failure
// contract: a hard rename is only defensible if the dead end says what to do.
func TestRetiredExtHintNamesReplacementAndCommand(t *testing.T) {
	tests := []struct {
		path        string
		wantsSubstr []string
	}{
		{"seeds/cli-nb.kmb", []string{".kmb", ".memory.json", "kapi memory export"}},
		{"seeds/terms.ktb", []string{".ktb", ".terms.json", "kapi terms export"}},
		{"i18n/en.kbf", []string{".kbf", ".kbf.json", "kapi extract"}},
		{"old/catalog.klf", []string{".klf", ".kbf.json", "kapi extract"}},
		{"old/tm.klftm", []string{".klftm", ".memory.json"}},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			hint := format.RetiredExtHint(tt.path)
			require.NotEmpty(t, hint, "a retired extension must explain itself")
			for _, want := range tt.wantsSubstr {
				assert.Contains(t, hint, want)
			}
		})
	}
}

// TestWithdrawnArchiveHintDoesNotPromiseARename separates the two kinds of
// retirement: .kmz/.ktz were removed outright, so the message must not read as
// if the file merely needs renaming.
func TestWithdrawnArchiveHintDoesNotPromiseARename(t *testing.T) {
	for _, path := range []string{"snap/tm.kmz", "snap/terms.ktz"} {
		hint := format.RetiredExtHint(path)
		require.NotEmpty(t, hint)
		assert.Contains(t, hint, "withdrawn")
		assert.NotContains(t, hint, "was renamed to")
	}
}

func TestRetiredExtHintSilentOnLiveExtensions(t *testing.T) {
	for _, path := range []string{
		"i18n/en.kbf.json", "archive.kpz", "l10n/tm/cli-nb.memory.json",
		"l10n/terms.json", "doc.xliff", "notes.md", "noext",
	} {
		assert.Empty(t, format.RetiredExtHint(path), "%s is a live path", path)
	}
}

func TestErrorWithRetiredExtHint(t *testing.T) {
	base := errors.New("unknown format")

	wrapped := format.ErrorWithRetiredExtHint(base, "seeds/cli-nb.kmb")
	require.Error(t, wrapped)
	assert.ErrorIs(t, wrapped, base, "the original error must stay inspectable")
	assert.Contains(t, wrapped.Error(), ".memory.json")

	assert.Equal(t, base, format.ErrorWithRetiredExtHint(base, "seeds/cli-nb.memory.json"),
		"a live path must not be decorated")
	assert.NoError(t, format.ErrorWithRetiredExtHint(nil, "seeds/cli-nb.kmb"))
}
