package termcandidate

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

func TestSpreadOrder(t *testing.T) {
	assert.Nil(t, SpreadOrder(0))
	assert.Equal(t, []int{0}, SpreadOrder(1))
	assert.Equal(t, []int{0, 2, 1, 3}, SpreadOrder(4))
	assert.Equal(t, []int{0, 5, 2, 7, 1, 3, 6, 8, 4, 9}, SpreadOrder(10))
	for n := 1; n < 60; n++ {
		order := SpreadOrder(n)
		require.Len(t, order, n)
		seen := map[int]bool{}
		for _, i := range order {
			seen[i] = true
		}
		assert.Len(t, seen, n, "every index once for n=%d", n)
	}
}

// itemText makes a fetch that answers each item with a fixed number of runes
// and records the order items were read in.
func itemText(runes int, read *[]string) FetchFunc {
	return func(_ context.Context, item string, maxRunes int) (string, error) {
		*read = append(*read, item)
		return truncateRunes(strings.Repeat("x", runes), maxRunes), nil
	}
}

func items(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s%02d", prefix, i)
	}
	return out
}

func TestSample_RoundRobinAcrossCollections(t *testing.T) {
	cols := []Collection{
		{Name: "docs", Items: items("d", 100)},
		{Name: "cli", Items: items("c", 3)},
		{Name: "mobile", Items: items("m", 10)},
	}
	var read []string
	segs, err := Sample(t.Context(), cols, itemText(100, &read), SampleOptions{TotalRunes: 900, ItemRunes: 100})
	require.NoError(t, err)
	require.Len(t, segs, 9)
	got := make([]string, len(segs))
	for i, s := range segs {
		got[i] = s.Collection
	}
	assert.Equal(t, []string{"docs", "cli", "mobile", "docs", "cli", "mobile", "docs", "cli", "mobile"}, got)
	// The docs collection is read at its first, middle and quarter items,
	// never only the front.
	assert.Equal(t, []string{"d00", "c00", "m00", "d50", "c01", "m05", "d25", "c02", "m02"}, read)
}

func TestSample_ExhaustedCollectionLeavesItsShareToOthers(t *testing.T) {
	cols := []Collection{
		{Name: "small", Items: items("s", 1)},
		{Name: "large", Items: items("l", 20)},
	}
	var read []string
	segs, err := Sample(t.Context(), cols, itemText(100, &read), SampleOptions{TotalRunes: 500, ItemRunes: 100})
	require.NoError(t, err)
	require.Len(t, segs, 5)
	assert.Equal(t, "small", segs[0].Collection)
	for _, s := range segs[1:] {
		assert.Equal(t, "large", s.Collection)
	}
}

func TestSample_SkipsEmptyItemsAndStopsWhenAllRead(t *testing.T) {
	cols := []Collection{{Name: "a", Items: []string{"empty", "full"}}}
	fetch := func(_ context.Context, item string, _ int) (string, error) {
		if item == "empty" {
			return "  ", nil
		}
		return "text", nil
	}
	segs, err := Sample(t.Context(), cols, fetch, SampleOptions{TotalRunes: 1000})
	require.NoError(t, err)
	require.Len(t, segs, 1)
	assert.Equal(t, "full", segs[0].Item)
}

func TestSample_CapsEachItem(t *testing.T) {
	cols := []Collection{{Name: "a", Items: []string{"long"}}}
	var read []string
	segs, err := Sample(t.Context(), cols, itemText(10000, &read), SampleOptions{TotalRunes: 5000, ItemRunes: 300})
	require.NoError(t, err)
	require.Len(t, segs, 1)
	assert.Len(t, segs[0].Text, 300)
}

func TestChunks(t *testing.T) {
	segs := []Segment{{Text: "aaaa"}, {Text: "bbbb"}, {Text: "cccc"}, {Text: strings.Repeat("d", 20)}}
	assert.Equal(t, []string{"aaaa\n\nbbbb", "cccc", "dddddddddd"}, Chunks(segs, 10, 0))
	assert.Equal(t, []string{"aaaa\n\nbbbb", "cccc"}, Chunks(segs, 10, 2))
	assert.Equal(t, []string{"aaaa\n\nbbbb\n\ncccc\n\n" + strings.Repeat("d", 20)}, Chunks(segs, 0, 0))
}

func TestCounter_EnglishInflectionsPerCollection(t *testing.T) {
	c := NewCounter("en-US", []string{"workspace", "Workspace", "content memory", " "})
	assert.Equal(t, []string{"workspace", "content memory"}, c.Terms())
	c.Add("docs", "Open a workspace. Workspaces hold projects.")
	c.Add("docs", "Your content memory is shared.")
	c.Add("cli", "kapi workspace list")
	c.Add("cli", "The pseudo-workspace is not a use.")
	c.Add("app", "Nothing here")

	r := c.Reach("WORKSPACE")
	assert.Equal(t, 2, r.Blocks)
	assert.Equal(t, []CollectionCount{{Name: "cli", Blocks: 1}, {Name: "docs", Blocks: 1}}, r.Collections)
	assert.Equal(t, Reach{Blocks: 1, Collections: []CollectionCount{{Name: "docs", Blocks: 1}}}, c.Reach("content memory"))
	assert.Equal(t, Reach{}, c.Reach("missing"))
}

func TestCounter_OtherLanguagesMatchTheSpelling(t *testing.T) {
	c := NewCounter("nb", []string{"løsning"})
	c.Add("docs", "En løsning for alt.")
	c.Add("docs", "Løsningen er klar.")
	assert.Equal(t, 1, c.Reach("løsning").Blocks)
}

func TestScope(t *testing.T) {
	cols := []Collection{
		{Name: "neokapi-docs", Coordinates: map[string]string{"brand": "neokapi", "product": "neokapi", "channel": "docs"}},
		{Name: "neokapi-cli", Coordinates: map[string]string{"brand": "neokapi", "product": "neokapi", "channel": "cli"}},
		{Name: "bowrain-app", Coordinates: map[string]string{"brand": "neokapi", "product": "bowrain", "channel": "app"}},
	}
	reach := func(names ...string) Reach {
		r := Reach{}
		for _, n := range names {
			r.Blocks++
			r.Collections = append(r.Collections, CollectionCount{Name: n, Blocks: 1})
		}
		return r
	}
	assert.Equal(t, map[string]string{"product": "neokapi", "channel": "docs"}, Scope(reach("neokapi-docs"), cols))
	assert.Equal(t, map[string]string{"product": "neokapi"}, Scope(reach("neokapi-docs", "neokapi-cli"), cols))
	assert.Nil(t, Scope(reach("neokapi-docs", "bowrain-app"), cols), "across products holds at the project")
	assert.Nil(t, Scope(Reach{}, cols))
	assert.Nil(t, Scope(reach("neokapi-docs"), cols[:1]), "a project of one collection has nothing narrower")
	assert.Nil(t, Scope(reach(""), append(cols, Collection{Name: ""})), "items outside a collection sit at the project")
}

func TestReachRoundTripsThroughProperties(t *testing.T) {
	r := Reach{Blocks: 7, Collections: []CollectionCount{{Name: "docs", Blocks: 5}, {Name: "cli", Blocks: 2}}}
	var cp terms.Concept
	r.Apply(&cp)
	assert.Equal(t, "7", cp.Properties[PropReachBlocks])
	assert.Equal(t, "docs=5,cli=2", cp.Properties[PropReachCollections])
	got, ok := ReachOf(cp)
	require.True(t, ok)
	assert.Equal(t, r, got)

	_, ok = ReachOf(terms.Concept{})
	assert.False(t, ok)
}

func TestFile_DedupesAndRemembersRejections(t *testing.T) {
	ctx := t.Context()
	tb := terms.NewInMemoryStore()
	now := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID: "kept", Terms: []terms.Term{{Text: "Workspace", Locale: "en", Status: model.TermPreferred}},
	}))
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID: "other-project", ProjectID: "p2", Terms: []terms.Term{{Text: "flow", Locale: "en", Status: model.TermPreferred}},
	}))

	cands := []Candidate{
		{Term: "workspace", Definition: "dup of a kept concept"},
		{Term: "Flow", Definition: "a run of tools", Reach: Reach{Blocks: 3, Collections: []CollectionCount{{Name: "docs", Blocks: 3}}},
			Coordinates: map[string]string{"product": "neokapi"}},
		{Term: "Synergy"},
		{Term: "flow"},
	}
	res, err := File(ctx, tb, "p1", "en-US", cands, map[string]bool{"synergy": true}, ScanIdentity, now)
	require.NoError(t, err)
	assert.Equal(t, []string{"workspace"}, res.Existing, "a repeat within one scan is dropped")
	assert.Equal(t, []string{"Synergy"}, res.Rejected)
	require.Len(t, res.Filed, 1)

	cp, ok, err := tb.GetConcept(ctx, res.Filed[0])
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "p1", cp.ProjectID)
	assert.Equal(t, "a run of tools", cp.Definition)
	assert.Equal(t, []terms.Term{{Text: "Flow", Locale: "en-US", Status: model.TermProposed}}, cp.Terms)
	assert.Equal(t, map[string]string{"product": "neokapi"}, cp.Coordinates())
	assert.Equal(t, ScanIdentity, cp.Properties[PropProposedBy])
	assert.True(t, IsCandidate(cp, "en"))
	r, _ := ReachOf(cp)
	assert.Equal(t, 3, r.Blocks)

	// A later scan refreshes the reach of the candidate it filed.
	cands[1].Reach = Reach{Blocks: 9, Collections: []CollectionCount{{Name: "docs", Blocks: 9}}}
	cands[1].Coordinates = nil
	res, err = File(ctx, tb, "p1", "en", cands[1:2], nil, ScanIdentity, now.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, []string{cp.ID}, res.Refreshed)
	cp, _, _ = tb.GetConcept(ctx, cp.ID)
	r, _ = ReachOf(cp)
	assert.Equal(t, 9, r.Blocks)
	assert.Nil(t, cp.Coordinates())

	// Once kept, the scan leaves it alone.
	require.True(t, Promote(&cp, "en"))
	assert.Equal(t, model.TermPreferred, cp.Terms[0].Status)
	assert.False(t, IsCandidate(cp, "en"))
	require.NoError(t, tb.AddConcept(ctx, cp))
	res, err = File(ctx, tb, "p1", "en", cands[1:2], nil, ScanIdentity, now)
	require.NoError(t, err)
	assert.Equal(t, []string{"Flow"}, res.Existing)
}

func TestRejectedList(t *testing.T) {
	props := map[string]string{}
	assert.True(t, AddRejected(props, " Synergy "))
	assert.False(t, AddRejected(props, "synergy"))
	assert.True(t, AddRejected(props, "leverage"))
	assert.False(t, AddRejected(props, ""))
	assert.Equal(t, "leverage\nsynergy", props[ProjectPropRejected])
	assert.Equal(t, map[string]bool{"leverage": true, "synergy": true}, Rejected(props))
	assert.Empty(t, Rejected(nil))
}
