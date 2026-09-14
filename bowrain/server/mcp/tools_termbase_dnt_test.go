package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/terms"
)

type singleTermsResolver struct{ tb terms.Store }

func (r singleTermsResolver) GetTB(string) (terms.Store, error) { return r.tb, nil }

type recordingProposer struct {
	workspace string
	concepts  []terms.Concept
}

func (p *recordingProposer) ProposeConcept(_ context.Context, workspace, _ string, concept terms.Concept) (string, error) {
	p.workspace = workspace
	p.concepts = append(p.concepts, concept)
	return "cs-proposed", nil
}

func newTermAddServer(t *testing.T, opts ...Option) (*MCPServer, terms.Store) {
	t.Helper()
	tb, err := terms.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = tb.Close() })
	ms, err := NewMCPServerWithStore(&memVoiceStore{}, nil, Config{}, append([]Option{WithTermsResolver(singleTermsResolver{tb: tb})}, opts...)...)
	require.NoError(t, err)
	return ms, tb
}

func termTexts(t *testing.T, tb terms.Store) []string {
	t.Helper()
	all, err := tb.Concepts(t.Context())
	require.NoError(t, err)
	var texts []string
	for _, c := range all {
		require.NotEmpty(t, c.Terms)
		texts = append(texts, c.Terms[0].Text)
	}
	return texts
}

// TestTermAdd_DoNotTranslateIsProposed: a term marked do-not-translate is a
// governed creation, so term_add proposes it through the change-set proposer
// and writes nothing for it. The ordinary term beside it is added as before.
func TestTermAdd_DoNotTranslateIsProposed(t *testing.T) {
	proposer := &recordingProposer{}
	ms, tb := newTermAddServer(t, WithChangeSetProposer(proposer))

	_, out, err := ms.handleTermAdd(t.Context(), nil, termAddInput{
		WorkspaceID: "acme",
		Terms: []termAddEntry{
			{Term: "kapi", Locale: "en", DoNotTranslate: true},
			{Term: "dashboard", Locale: "en"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, out.Added)
	assert.Equal(t, 1, out.Proposed)
	assert.Equal(t, []string{"cs-proposed"}, out.ChangeSetIDs)
	assert.Equal(t, []string{"dashboard"}, termTexts(t, tb), "only the ordinary term is written")

	require.Len(t, proposer.concepts, 1)
	assert.Equal(t, "acme", proposer.workspace)
	assert.True(t, proposer.concepts[0].DoNotTranslate)
	assert.NotEmpty(t, proposer.concepts[0].ID)
	require.NotEmpty(t, proposer.concepts[0].Terms)
	assert.Equal(t, "kapi", proposer.concepts[0].Terms[0].Text)
}

// TestTermAdd_DoNotTranslateWithoutAProposerIsRefused: a server that cannot open
// a change-set refuses a do-not-translate term rather than writing it.
func TestTermAdd_DoNotTranslateWithoutAProposerIsRefused(t *testing.T) {
	ms, tb := newTermAddServer(t)

	_, _, err := ms.handleTermAdd(t.Context(), nil, termAddInput{
		WorkspaceID: "acme",
		Terms:       []termAddEntry{{Term: "kapi", Locale: "en", DoNotTranslate: true}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "change-set")
	assert.Empty(t, termTexts(t, tb), "nothing is written")
}
