package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
)

// One translation stored once per origin is one lookup result naming every
// entry and origin. A different target stays a result of its own.
func TestMemoryLookupEntriesMergesOneTranslation(t *testing.T) {
	entry := func(id, target, origin string) memory.Entry {
		return memory.Entry{
			ID: id,
			Variants: map[model.LocaleID][]model.Run{
				"en-GB": {{Text: &model.TextRun{Text: "Berth plan"}}},
				"nb":    {{Text: &model.TextRun{Text: target}}},
			},
			Origins: []memory.Origin{{Source: origin}},
		}
	}
	matches := []memory.Match{
		{Entry: entry("record:1", "Kaiplan", "record"), Score: 1, MatchType: memory.MatchExact},
		{Entry: entry("merge:1", "Kaiplan", "merge"), Score: 1, MatchType: memory.MatchExact},
		{Entry: entry("import:1", "Liggeplan", "import"), Score: 0.9, MatchType: memory.MatchFuzzy},
	}
	got := MemoryLookupEntries(matches, "en-GB", "nb")
	require.Len(t, got, 2)
	assert.Equal(t, "Kaiplan", got[0].Target)
	assert.Equal(t, "record:1", got[0].EntryID)
	assert.Equal(t, []string{"record:1", "merge:1"}, got[0].EntryIDs)
	assert.Equal(t, []string{"merge", "record"}, got[0].Origins)
	assert.Equal(t, "Liggeplan", got[1].Target)
	assert.Equal(t, []string{"import"}, got[1].Origins)
}

// Two different pairs whose texts would join to the same string with a NUL
// delimiter stay two results.
func TestMemoryLookupEntriesKeepsPairsWithANulApart(t *testing.T) {
	pair := func(id, source, target string) memory.Match {
		return memory.Match{Entry: memory.Entry{
			ID: id,
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: source}}},
				"nb": {{Text: &model.TextRun{Text: target}}},
			},
		}, Score: 1, MatchType: memory.MatchExact}
	}
	got := MemoryLookupEntries([]memory.Match{
		pair("a", "x\x00y", "z"),
		pair("b", "x", "y\x00z"),
	}, "en", "nb")
	require.Len(t, got, 2)
	assert.Equal(t, []string{"a"}, got[0].EntryIDs)
	assert.Equal(t, []string{"b"}, got[1].EntryIDs)
}
