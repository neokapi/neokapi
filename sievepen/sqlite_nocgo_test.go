//go:build !cgo

package sievepen_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Under no-cgo builds the FTS5 word-search table (tm_variant_search) uses the
// built-in unicode61 tokenizer instead of the cgo-only ICU tokenizer. unicode61
// splits on whitespace and punctuation but does not segment scripts without
// explicit word boundaries (CJK, Thai), so a contiguous string becomes a single
// token. These tests assert that behaviour, mirroring the ICU-segmentation
// cases in sqlite_icu_test.go.

// TestSQLiteTM_SearchCJK_Unicode61 documents that unicode61 treats a
// space-less CJK string as one token: a sub-word query does not match, but a
// query for the whole token does.
func TestSQLiteTM_SearchCJK_Unicode61(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"zh-CN": {{Text: &model.TextRun{Text: "中国经济发展报告"}}},
			"en":    {{Text: &model.TextRun{Text: "China economic development report"}}},
		},
		HintSrcLang: "zh-CN",
	}))

	// Sub-word search does NOT match under unicode61 (no CJK segmentation).
	_, total := tm.SearchEntries("经济", "zh-CN", "", 0, 10)
	assert.Equal(t, 0, total, "unicode61 does not segment CJK sub-words")

	// Whole-string search matches, since the string is a single token.
	entries, total := tm.SearchEntries("中国经济发展报告", "zh-CN", "", 0, 10)
	assert.Equal(t, 1, total, "unicode61 matches the whole CJK token")
	if len(entries) > 0 {
		assert.Equal(t, "e1", entries[0].ID)
	}
}

func TestSQLiteTM_SearchThai_Unicode61(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"th-TH": {{Text: &model.TextRun{Text: "การทดสอบภาษาไทยในระบบค้นหา"}}},
			"en":    {{Text: &model.TextRun{Text: "Testing Thai language in the search system"}}},
		},
		HintSrcLang: "th-TH",
	}))

	// Sub-word search does NOT match under unicode61 (no Thai segmentation).
	_, total := tm.SearchEntries("ภาษา", "th-TH", "", 0, 10)
	assert.Equal(t, 0, total, "unicode61 does not segment Thai sub-words")

	// Whole-string search matches.
	entries, total := tm.SearchEntries("การทดสอบภาษาไทยในระบบค้นหา", "th-TH", "", 0, 10)
	assert.Equal(t, 1, total, "unicode61 matches the whole Thai token")
	if len(entries) > 0 {
		assert.Equal(t, "e1", entries[0].ID)
	}
}

// TestSQLiteTM_SearchSpaceSeparated_Unicode61 confirms that word-search still
// works for scripts with explicit word boundaries under unicode61.
func TestSQLiteTM_SearchSpaceSeparated_Unicode61(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"ja-JP": {{Text: &model.TextRun{Text: "日本語 の テスト です"}}},
			"en":    {{Text: &model.TextRun{Text: "This is a Japanese test"}}},
		},
		HintSrcLang: "ja-JP",
	}))

	entries, total := tm.SearchEntries("テスト", "ja-JP", "", 0, 10)
	assert.Equal(t, 1, total, "unicode61 splits on whitespace and finds テスト")
	if len(entries) > 0 {
		assert.Equal(t, "e1", entries[0].ID)
	}
}
