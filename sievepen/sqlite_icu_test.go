//go:build cgo

package sievepen_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests require the FTS5 ICU tokenizer, which is a cgo-only extension
// (statically linked via core/storage/icu_tokenizer.go). Under no-cgo builds
// the word-search table uses unicode61, which does not segment scripts without
// explicit word boundaries; see sqlite_nocgo_test.go for the matching
// unicode61 expectations.

// TestSQLiteTM_SearchCJK verifies that the ICU tokenizer correctly segments
// CJK text (no spaces between words). With the unicode61 tokenizer, the
// entire string would be one token and sub-word searches would fail.
func TestSQLiteTM_SearchCJK(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(context.Background(), sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"zh-CN": {{Text: &model.TextRun{Text: "中国经济发展报告"}}},
			"en":    {{Text: &model.TextRun{Text: "China economic development report"}}},
		},
		HintSrcLang: "zh-CN",
	}))
	require.NoError(t, tm.Add(context.Background(), sievepen.TMEntry{
		ID: "e2",
		Variants: map[model.LocaleID][]model.Run{
			"zh-CN": {{Text: &model.TextRun{Text: "国际贸易协定"}}},
			"en":    {{Text: &model.TextRun{Text: "International trade agreement"}}},
		},
		HintSrcLang: "zh-CN",
	}))

	// Search for "经济" (economy) in Chinese — ICU segments at word boundaries.
	entries, total, _ := tm.SearchEntries(context.Background(), sievepen.SearchParams{Query: "经济", AnyLocale: "zh-CN", RequireLocale: "", Offset: 0, Limit: 10})
	assert.Equal(t, 1, total, "ICU should segment Chinese and find 经济")
	if len(entries) > 0 {
		assert.Equal(t, "e1", entries[0].ID)
	}

	// Search for "贸易" (trade) — should find e2.
	entries, total, _ = tm.SearchEntries(context.Background(), sievepen.SearchParams{Query: "贸易", AnyLocale: "zh-CN", RequireLocale: "", Offset: 0, Limit: 10})
	assert.Equal(t, 1, total)
	if len(entries) > 0 {
		assert.Equal(t, "e2", entries[0].ID)
	}
}

func TestSQLiteTM_SearchJapanese(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(context.Background(), sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"ja-JP": {{Text: &model.TextRun{Text: "日本語のテストです"}}},
			"en":    {{Text: &model.TextRun{Text: "This is a Japanese test"}}},
		},
		HintSrcLang: "ja-JP",
	}))

	entries, total, _ := tm.SearchEntries(context.Background(), sievepen.SearchParams{Query: "テスト", AnyLocale: "ja-JP", RequireLocale: "", Offset: 0, Limit: 10})
	assert.Equal(t, 1, total, "ICU should segment Japanese and find テスト")
	if len(entries) > 0 {
		assert.Equal(t, "e1", entries[0].ID)
	}
}

func TestSQLiteTM_SearchThai(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(context.Background(), sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"th-TH": {{Text: &model.TextRun{Text: "การทดสอบภาษาไทยในระบบค้นหา"}}},
			"en":    {{Text: &model.TextRun{Text: "Testing Thai language in the search system"}}},
		},
		HintSrcLang: "th-TH",
	}))

	entries, total, _ := tm.SearchEntries(context.Background(), sievepen.SearchParams{Query: "ภาษา", AnyLocale: "th-TH", RequireLocale: "", Offset: 0, Limit: 10})
	assert.Equal(t, 1, total, "ICU should segment Thai and find ภาษา")
	if len(entries) > 0 {
		assert.Equal(t, "e1", entries[0].ID)
	}
}
