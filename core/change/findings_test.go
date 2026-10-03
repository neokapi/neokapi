package change_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// What the commit check found is the document's, in one place: an empty list
// when the check ran and found nothing, every finding the edit introduced,
// failing or not, when it found something, and nothing at all when no check
// ran.
func TestService_FindingsArePresentWheneverTheCheckRan(t *testing.T) {
	newHome := func() *memHome {
		return newMemHome(map[string][]memBlock{"a": {textBlock("one", "Clean text")}})
	}
	findings := func(t *testing.T, res *change.Result) (json.RawMessage, bool) {
		t.Helper()
		raw, err := json.Marshal(res)
		require.NoError(t, err)
		var wire struct {
			Docs []map[string]json.RawMessage `json:"docs"`
		}
		require.NoError(t, json.Unmarshal(raw, &wire))
		require.Len(t, wire.Docs, 1)
		f, ok := wire.Docs[0]["findings"]
		return f, ok
	}

	t.Run("a clean check", func(t *testing.T) {
		svc := newMemService(newHome(), change.WithCommitCheck(&wordCheck{word: "utilize"}))
		b := readBlock(t, svc, "a", "one")
		res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{edit(b.Ref, b.Rev, "Clean words")}}, svcPerson)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status)
		got, ok := findings(t, res)
		require.True(t, ok, "the check ran, so the document lists its findings")
		assert.JSONEq(t, `[]`, string(got))
	})

	t.Run("no check", func(t *testing.T) {
		svc := newMemService(newHome())
		b := readBlock(t, svc, "a", "one")
		res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{edit(b.Ref, b.Rev, "Clean words")}}, svcPerson)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status)
		_, ok := findings(t, res)
		assert.False(t, ok, "no check ran, so the result says nothing about findings")
	})

	t.Run("a finding that reports without failing", func(t *testing.T) {
		svc := newMemService(newHome(), change.WithCommitCheck(&wordCheck{word: "utilize", report: true}))
		b := readBlock(t, svc, "a", "one")
		res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{edit(b.Ref, b.Rev, "Now utilize it")}}, svcPerson)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status, "a finding that does not fail lands")
		require.Len(t, res.Docs[0].Findings, 1)
		assert.False(t, res.Docs[0].Findings[0].Fails)
		assert.Empty(t, res.Ops[0].Findings)
	})
}

// A finding names the span it found and, for a term rule, the wording to use
// instead, so a sender rewrites it without reading the message.
func TestFindingRange(t *testing.T) {
	a := model.SpanAnchor(model.RunPos{Run: 0, Offset: 4}, model.RunPos{Run: 0, Offset: 11})
	got := change.FindingRange(&a)
	require.NotNil(t, got)
	assert.Equal(t, model.RunPos{Run: 0, Offset: 4}, got.Start)
	assert.Equal(t, model.RunPos{Run: 0, Offset: 11}, got.End)
	block := model.BlockAnchor()
	assert.Nil(t, change.FindingRange(&block), "an anchor on the whole block names no span")
	assert.Nil(t, change.FindingRange(nil))
}
