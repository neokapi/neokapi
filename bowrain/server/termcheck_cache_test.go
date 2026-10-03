package server

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// revisionedTestTerms is a terms store that names a revision, as the
// PostgreSQL store does, and counts the times its whole terms are read.
type revisionedTestTerms struct {
	*testTermStore
	revision string
	reads    int
	failRead bool
}

func (r *revisionedTestTerms) Revision(context.Context) (string, error) { return r.revision, nil }

func (r *revisionedTestTerms) Concepts(ctx context.Context) ([]terms.Concept, error) {
	r.reads++
	if r.failRead {
		return nil, errors.New("the terms are unreadable")
	}
	return r.testTermStore.Concepts(ctx)
}

func termConcept(id, text string) terms.Concept {
	return terms.Concept{ID: id, Source: terms.TermSourceTerminology,
		Terms: []terms.Term{{Text: text, Locale: "en", Status: model.TermPreferred}}}
}

// TestTermSnapshotCache_ReadsTheTermsOncePerRevision pins that the term gate
// reads a workspace's whole terms once for each revision they are at: a call
// at the revision it read reuses the snapshot, a call after a write reads the
// terms again and sees the write, a failed read is not kept, and a store that
// names no revision is read every time.
func TestTermSnapshotCache_ReadsTheTermsOncePerRevision(t *testing.T) {
	ctx := t.Context()
	tb := &revisionedTestTerms{testTermStore: &testTermStore{terms.NewInMemoryStore()}, revision: "r1"}
	require.NoError(t, tb.AddConcept(ctx, termConcept("c-kapi", "kapi")))
	var c termSnapshotCache

	snap1, fp1 := c.snapshot(ctx, "ws", tb)
	require.NotNil(t, snap1)
	snap2, fp2 := c.snapshot(ctx, "ws", tb)
	assert.Same(t, snap1, snap2, "the same revision reuses the snapshot")
	assert.Equal(t, fp1, fp2)
	assert.Equal(t, 1, tb.reads, "the terms are read once for the revision")

	require.NoError(t, tb.AddConcept(ctx, termConcept("c-shop", "shop")))
	tb.revision = "r2"
	snap3, fp3 := c.snapshot(ctx, "ws", tb)
	assert.Equal(t, 2, tb.reads, "a new revision reads the terms again")
	assert.NotEqual(t, fp1, fp3, "and the digest names the write")
	all, err := snap3.Concepts(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	tb.revision = "r3"
	tb.failRead = true
	snap4, fp4 := c.snapshot(ctx, "ws", tb)
	assert.Nil(t, snap4, "a failed read governs nothing")
	assert.Empty(t, fp4)
	tb.failRead = false
	snap5, _ := c.snapshot(ctx, "ws", tb)
	assert.NotNil(t, snap5, "and is not kept: the next call reads again")
	assert.Equal(t, 4, tb.reads)

	tb.revision = ""
	c.snapshot(ctx, "ws", tb)
	c.snapshot(ctx, "ws", tb)
	assert.Equal(t, 6, tb.reads, "a store with no revision recorded yet is read every time")

	other := &revisionedTestTerms{testTermStore: &testTermStore{terms.NewInMemoryStore()}, revision: "r3"}
	require.NoError(t, other.AddConcept(ctx, termConcept("c-other", "other")))
	snapOther, _ := c.snapshot(ctx, "ws-other", other)
	otherAll, err := snapOther.Concepts(ctx)
	require.NoError(t, err)
	require.Len(t, otherAll, 1, "a revision is kept per workspace")
	assert.Equal(t, "c-other", otherAll[0].ID)
}
