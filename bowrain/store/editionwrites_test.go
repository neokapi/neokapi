package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// pushWrites applies one push: blocks into en.json, then the edition writes
// beside them, on one transaction.
func pushWrites(t *testing.T, s *PostgresStore, projectID, author string, blocks []*model.Block, writes []venue.EditionWrite) int {
	t.Helper()
	n := 0
	require.NoError(t, s.ApplyPush(t.Context(), func(tx platstore.PushApplier) error {
		if len(blocks) > 0 {
			if err := tx.StoreBlocksForItem(t.Context(), projectID, "main", "en.json", blocks); err != nil {
				return err
			}
		}
		var err error
		n, err = tx.RecordEditionWrites(t.Context(), projectID, "main", author, writes)
		return err
	}))
	return n
}

// storedRow is the stored block for unit in en.json.
func storedRow(t *testing.T, s *PostgresStore, projectID, unit string) *venue.StoredBlock {
	t.Helper()
	blocks, err := s.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: projectID, Stream: "main", ItemName: "en.json"})
	require.NoError(t, err)
	for _, b := range blocks {
		if b.SourceID == unit {
			return b
		}
	}
	t.Fatalf("en.json holds no %s", unit)
	return nil
}

// nbRevision is the revision of the Norwegian translation the venue holds for
// unit, as a checkout's block history names it.
func nbRevision(t *testing.T, s *PostgresStore, projectID, unit string) string {
	t.Helper()
	return model.EditionRevision(storedRow(t, s, projectID, unit).Block, model.EditionKey{Locale: "nb"})
}

func nbTally(t *testing.T, s *PostgresStore, projectID string) platstore.DecisionBasisTally {
	t.Helper()
	tallies, err := s.TallyDecisionBasis(t.Context(), projectID, "main")
	require.NoError(t, err)
	var out platstore.DecisionBasisTally
	for _, tl := range tallies {
		if tl.Variant == "nb" {
			out.Stale += tl.Stale
			out.BasisUnknown += tl.BasisUnknown
			out.Owed += tl.Owed
		}
	}
	return out
}

// nbDerived is the derivation the Norwegian translation of unit records, or
// nil when it records none.
func nbDerived(t *testing.T, s *PostgresStore, projectID, unit string) *model.Derivation {
	t.Helper()
	d, ok := storedRow(t, s, projectID, unit).Block.Derivation(model.EditionKey{Locale: "nb"})
	if !ok {
		return nil
	}
	return &d
}

// nbHistory is the block history of the Norwegian translation of unit, the
// latest entry first.
func nbHistory(t *testing.T, s *PostgresStore, projectID, unit string) []platstore.BlockHistoryEntry {
	t.Helper()
	entries, err := s.GetBlockHistory(t.Context(), projectID, "main", storedRow(t, s, projectID, unit).ID, "nb", 0)
	require.NoError(t, err)
	return entries
}

// A translation a run on a checkout made reaches the venue with the basis the
// run recorded on its edition, and is graded stale when the source moves, as a
// draft of the venue's own is. The ledger gains no record: a basis decides
// nothing.
func TestRecordEditionWrites_GradesALocalRunsTranslationAgainstItsSource(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	pushWrites(t, s, p.ID, "", []*model.Block{
		blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated),
		blockWithText("farewell", "Goodbye"),
	}, nil)
	require.Nil(t, nbDerived(t, s, p.ID, "greeting"))

	write := venue.EditionWrite{ItemName: "en.json", Unit: "greeting", Variant: "nb", Revision: nbRevision(t, s, p.ID, "greeting"),
		Basis: sourceRevision("Hello"), Writer: venue.WriterTool, Origin: "flow:up", GoverningFingerprint: "fp-1"}
	n := pushWrites(t, s, p.ID, "u-pusher", nil, []venue.EditionWrite{
		write,
		// The checkout holds this translation and the venue does not.
		{ItemName: "en.json", Unit: "farewell", Variant: "nb", Revision: "r:0123456789abcdef",
			Basis: sourceRevision("Goodbye"), Writer: venue.WriterTool, Origin: "flow:up"},
	})
	assert.Equal(t, 2, n)

	got := nbDerived(t, s, p.ID, "greeting")
	require.NotNil(t, got, "the basis lands on the edition")
	assert.Equal(t, sourceRevision("Hello"), got.Rev)
	assert.True(t, got.From.IsZero(), "made from the edition the document is written in")
	assert.Equal(t, model.TargetStatusTranslated, targetStatus(t, s, p.ID, "en.json", "greeting"), "and decides nothing")
	assert.Empty(t, listDecisions(t, s, p.ID), "the ledger holds no record that carries a basis alone")
	drafts, err := s.ListDraftBases(t.Context(), p.ID, "main")
	require.NoError(t, err)
	assert.Empty(t, drafts, "a draft mark lands on a row and creates none")
	assert.Zero(t, nbTally(t, s, p.ID).Stale, "the translation renders the source the venue holds")

	// The source moves.
	pushWrites(t, s, p.ID, "", []*model.Block{blockWithText("greeting", "Hello there")}, nil)
	tally := nbTally(t, s, p.ID)
	assert.Equal(t, 1, tally.Stale, "the translation a local run made reads stale on the venue")
	assert.Equal(t, 1, tally.Owed)

	// Sending the same write again changes nothing.
	n = pushWrites(t, s, p.ID, "u-pusher", nil, []venue.EditionWrite{write})
	assert.Equal(t, 1, n)
	assert.Equal(t, got, nbDerived(t, s, p.ID, "greeting"))
}

// A write that names no source for the translation it left clears the basis
// the edition recorded, rather than leaving the basis of a translation the
// write replaced. A write that says nothing about the source leaves it.
func TestRecordEditionWrites_ClearsABasisTheWriteDoesNotCarry(t *testing.T) {
	tests := []struct {
		name  string
		write venue.EditionWrite
		keeps bool
	}{
		{name: "an edit by hand", write: venue.EditionWrite{Writer: venue.WriterPerson, Origin: "desktop"}},
		{name: "an edit made outside kapi", write: venue.EditionWrite{Writer: venue.WriterExternal, Origin: "observed"}},
		{name: "a translation a pull brought in", write: venue.EditionWrite{Writer: venue.WriterTool, Origin: "pull"}, keeps: true},
		{name: "a translation a merge brought in", write: venue.EditionWrite{Writer: venue.WriterPerson, Origin: "merge"}, keeps: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			p := createTestProject(t, s)
			b := blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated)
			b.SetDerivation(model.EditionKey{Locale: "nb"}, &model.Derivation{Rev: sourceRevision("Hello")})
			pushWrites(t, s, p.ID, "", []*model.Block{b}, nil)
			require.NotNil(t, nbDerived(t, s, p.ID, "greeting"))

			w := tc.write
			w.ItemName, w.Unit, w.Variant, w.Revision = "en.json", "greeting", "nb", nbRevision(t, s, p.ID, "greeting")
			pushWrites(t, s, p.ID, "u-pusher", nil, []venue.EditionWrite{w})

			if tc.keeps {
				assert.NotNil(t, nbDerived(t, s, p.ID, "greeting"), "the write says nothing new about the source")
				return
			}
			assert.Nil(t, nbDerived(t, s, p.ID, "greeting"), "made from no recorded source")
			assert.Equal(t, model.TargetStatusTranslated, targetStatus(t, s, p.ID, "en.json", "greeting"), "the rest of the edition stays")
			assert.Empty(t, listDecisions(t, s, p.ID))

			pushWrites(t, s, p.ID, "", []*model.Block{blockWithText("greeting", "Hello there")}, nil)
			assert.Zero(t, nbTally(t, s, p.ID).Stale, "nothing says what the translation renders, so nothing calls it stale")
		})
	}
}

// A push that carries a translation stores the derivation its edition carries,
// and one that carries none clears the derivation the venue held.
func TestApplyPush_APushedEditionReplacesItsDerivation(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	b := blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated)
	b.SetDerivation(model.EditionKey{Locale: "nb"}, &model.Derivation{Rev: sourceRevision("Hello")})
	pushWrites(t, s, p.ID, "", []*model.Block{b}, nil)
	require.NotNil(t, nbDerived(t, s, p.ID, "greeting"))

	pushWrites(t, s, p.ID, "", []*model.Block{blockWithTarget("greeting", "Hello", "Hallo", model.TargetStatusTranslated)}, nil)
	assert.Nil(t, nbDerived(t, s, p.ID, "greeting"), "the pushed edition records no derivation")
}

// The block history records the basis each revision of a translation
// carried: the derivation on the edition the push stored, and the basis the
// push's write gave it.
func TestRecordEditionWrites_RecordsTheBasisInTheBlockHistory(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)

	// A pushed edition that records its derivation.
	b := blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated)
	b.SetDerivation(model.EditionKey{Locale: "nb"}, &model.Derivation{Rev: sourceRevision("Hello")})
	pushWrites(t, s, p.ID, "", []*model.Block{b}, nil)
	history := nbHistory(t, s, p.ID, "greeting")
	require.Len(t, history, 1)
	assert.Equal(t, sourceRevision("Hello"), history[0].Basis)
	assert.Empty(t, history[0].BasisFrom, "the edition the document is written in")

	// A run's draft whose write names the source.
	drafted := blockWithTarget("greeting", "Hello", "Hallo", model.TargetStatusTranslated)
	pushWrites(t, s, p.ID, "u-pusher", []*model.Block{drafted}, []venue.EditionWrite{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Revision: model.EditionRevision(drafted, model.EditionKey{Locale: "nb"}),
		Basis:    sourceRevision("Hello"), Writer: venue.WriterTool, Origin: "flow:up",
	}})
	history = nbHistory(t, s, p.ID, "greeting")
	require.Len(t, history, 2)
	assert.Equal(t, "Hallo", history[0].Text)
	assert.Equal(t, sourceRevision("Hello"), history[0].Basis, "the basis the write gave the draft")

	// An edit by hand, which names no source.
	edited := blockWithTarget("greeting", "Hello", "Heisann", model.TargetStatusTranslated)
	edited.SetDerivation(model.EditionKey{Locale: "nb"}, &model.Derivation{Rev: sourceRevision("Hello")})
	pushWrites(t, s, p.ID, "u-pusher", []*model.Block{edited}, []venue.EditionWrite{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Revision: model.EditionRevision(edited, model.EditionKey{Locale: "nb"}),
		Writer:   venue.WriterPerson, Origin: "desktop",
	}})
	history = nbHistory(t, s, p.ID, "greeting")
	require.Len(t, history, 3)
	assert.Equal(t, "Heisann", history[0].Text)
	assert.Empty(t, history[0].Basis, "the write names no source for the edit")
	assert.Nil(t, nbDerived(t, s, p.ID, "greeting"))
	assert.Equal(t, sourceRevision("Hello"), history[1].Basis, "an earlier entry keeps the basis it recorded")
}

func TestRecordEditionWrites_LeavesWhatItDoesNotDescribe(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, s *PostgresStore, projectID string)
		write venue.EditionWrite
		check func(t *testing.T, s *PostgresStore, projectID string)
	}{
		{
			name:  "a write about another translation",
			write: venue.EditionWrite{Revision: "r:ffffffffffffffff", Basis: sourceRevision("Hello"), Writer: venue.WriterTool, Origin: "flow:up"},
			check: func(t *testing.T, s *PostgresStore, projectID string) {
				assert.Nil(t, nbDerived(t, s, projectID, "greeting"), "the venue holds another translation, and records nothing on it")
				assert.Empty(t, listDecisions(t, s, projectID))
			},
		},
		{
			name: "a decided unit",
			setup: func(t *testing.T, s *PostgresStore, projectID string) {
				_, err := s.UpsertUnitDecisions(t.Context(), projectID, "main", []venue.UnitDecision{{
					ItemName: "en.json", Unit: "greeting", Variant: "nb",
					Status: string(model.TargetStatusEstablished), ReviewState: venue.ReviewStateApproved,
					Revision: nbTextRevision("Hei"), Basis: sourceRevision("Hello"),
					DecidedBy: "u-reviewer", Updated: "2026-08-04T10:00:00Z",
				}})
				require.NoError(t, err)
			},
			write: venue.EditionWrite{Basis: sourceRevision("Hello again"), Writer: venue.WriterTool, Origin: "flow:up"},
			check: func(t *testing.T, s *PostgresStore, projectID string) {
				d := listDecisions(t, s, projectID)["en.json|greeting|nb"]
				assert.Equal(t, venue.ReviewStateApproved, d.ReviewState)
				assert.Equal(t, sourceRevision("Hello"), d.Basis, "a decision's basis is the decision's")
				drafts, err := s.ListDraftBases(t.Context(), projectID, "main")
				require.NoError(t, err)
				require.Len(t, drafts, 1)
				assert.Equal(t, sourceRevision("Hello again"), drafts[0].Basis, "the run's draft is marked beside it")
				got := nbDerived(t, s, projectID, "greeting")
				require.NotNil(t, got)
				assert.Equal(t, sourceRevision("Hello again"), got.Rev, "and the edition records what the run made it from")
			},
		},
		{
			// The venue made it, and its own record holds the source it was
			// made from; a basis the write carries names the source the
			// checkout held when it pulled.
			name:  "a translation a pull brought in, carrying a basis",
			write: venue.EditionWrite{Basis: sourceRevision("Hello"), Writer: venue.WriterTool, Origin: "pull"},
			check: func(t *testing.T, s *PostgresStore, projectID string) {
				assert.Nil(t, nbDerived(t, s, projectID, "greeting"), "no basis")
				drafts, err := s.ListDraftBases(t.Context(), projectID, "main")
				require.NoError(t, err)
				assert.Empty(t, drafts, "and no draft mark")
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			p := createTestProject(t, s)
			pushWrites(t, s, p.ID, "", []*model.Block{blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated)}, nil)
			if tc.setup != nil {
				tc.setup(t, s, p.ID)
			}
			w := tc.write
			w.ItemName, w.Unit, w.Variant = "en.json", "greeting", "nb"
			if w.Revision == "" {
				w.Revision = nbRevision(t, s, p.ID, "greeting")
			}
			pushWrites(t, s, p.ID, "u-pusher", nil, []venue.EditionWrite{w})
			tc.check(t, s, p.ID)
		})
	}
}

// Separation of duties reads who last wrote a translation from the block
// history. A translation the pusher wrote by hand on their checkout lands with
// them as its author; one a run produced, or a pull or a merge brought in,
// lands with none.
func TestRecordEditionWrites_NamesThePusherAsTheAuthorOfWhatTheyWrote(t *testing.T) {
	tests := []struct {
		writer, origin string
		author         string
	}{
		{writer: venue.WriterPerson, origin: "apply", author: "u-pusher"},
		{writer: venue.WriterPerson, origin: "desktop", author: "u-pusher"},
		{writer: venue.WriterPerson, origin: "browser", author: "u-pusher"},
		{writer: venue.WriterPerson, origin: "ksed", author: "u-pusher"},
		{writer: venue.WriterAgent, origin: "mcp", author: "u-pusher"},
		{writer: venue.WriterTool, origin: "flow:up"},
		{writer: venue.WriterTool, origin: "pull"},
		{writer: venue.WriterPerson, origin: "merge"},
		{writer: venue.WriterExternal, origin: "observed"},
	}
	for _, tc := range tests {
		t.Run(tc.writer+" through "+tc.origin, func(t *testing.T) {
			s := newTestStore(t)
			p := createTestProject(t, s)
			pushWrites(t, s, p.ID, "", []*model.Block{blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated)}, nil)

			// The push carries the new translation and how it was written.
			edited := blockWithTarget("greeting", "Hello", "Hallo", model.TargetStatusTranslated)
			require.NoError(t, s.ApplyPush(t.Context(), func(tx platstore.PushApplier) error {
				if err := tx.StoreBlocksForItem(t.Context(), p.ID, "main", "en.json", []*model.Block{edited}); err != nil {
					return err
				}
				_, err := tx.RecordEditionWrites(t.Context(), p.ID, "main", "u-pusher", []venue.EditionWrite{{
					ItemName: "en.json", Unit: "greeting", Variant: "nb",
					Revision: model.EditionRevision(edited, model.EditionKey{Locale: "nb"}),
					Writer:   tc.writer, Origin: tc.origin,
				}})
				return err
			}))

			row := storedRow(t, s, p.ID, "greeting")
			authors, err := s.LastTargetAuthors(t.Context(), p.ID, "main", []string{row.ID}, []string{"nb"})
			require.NoError(t, err)
			assert.Equal(t, tc.author, authors[platstore.TargetRef{BlockID: row.ID, Locale: "nb"}])
		})
	}
}

// writersOf is who the venue records as having written each Norwegian
// translation in en.json by hand, by unit.
func writersOf(t *testing.T, s *PostgresStore, projectID string) map[string]platstore.EditionWriter {
	t.Helper()
	rows, err := s.EditionWriters(t.Context(), projectID, "main", []string{"en.json"})
	require.NoError(t, err)
	out := map[string]platstore.EditionWriter{}
	for _, r := range rows {
		if r.Variant == "nb" {
			out[r.Unit] = r
		}
	}
	return out
}

// A translation written by hand on a checkout is recorded with its author
// whether or not the venue holds it, so an approval in a later push can be
// judged against it. The first pusher of a revision stays its author when
// another checkout sends the same write; a write of another revision by
// anybody else ends the claim.
func TestRecordEditionWrites_KeepsWhoWroteATranslationByHand(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	pushWrites(t, s, p.ID, "", []*model.Block{blockWithText("greeting", "Hello"), blockWithText("farewell", "Goodbye")}, nil)
	write := func(unit, rev, writer, origin string) venue.EditionWrite {
		return venue.EditionWrite{ItemName: "en.json", Unit: unit, Variant: "nb",
			Revision: rev, Writer: writer, Origin: origin}
	}

	pushWrites(t, s, p.ID, "u-ada", nil, []venue.EditionWrite{
		write("greeting", "r:1111111111111111", venue.WriterPerson, "apply"),
		write("farewell", "r:2222222222222222", venue.WriterTool, "flow:up"),
	})
	got := writersOf(t, s, p.ID)
	require.Len(t, got, 1, "a run's translation has no author")
	assert.Equal(t, platstore.EditionWriter{ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Revision: "r:1111111111111111", Author: "u-ada"}, got["greeting"], "the venue holds no translation of it, and keeps its author")

	// Another checkout, holding Ada's record, sends the same write.
	pushWrites(t, s, p.ID, "u-ben", nil, []venue.EditionWrite{write("greeting", "r:1111111111111111", venue.WriterPerson, "apply")})
	assert.Equal(t, "u-ada", writersOf(t, s, p.ID)["greeting"].Author, "the first pusher of a revision stays its author")

	// Ben rewrites it by hand.
	pushWrites(t, s, p.ID, "u-ben", nil, []venue.EditionWrite{write("greeting", "r:3333333333333333", venue.WriterAgent, "mcp")})
	assert.Equal(t, "u-ben", writersOf(t, s, p.ID)["greeting"].Author)

	// A run re-drafts it.
	pushWrites(t, s, p.ID, "u-ada", nil, []venue.EditionWrite{write("greeting", "r:4444444444444444", venue.WriterTool, "flow:up")})
	assert.Empty(t, writersOf(t, s, p.ID), "nobody wrote the run's draft by hand")
}

// A push that sends a run's write again finds the draft mark already saying
// it, and leaves the unit's record as it was.
func TestRecordEditionWrites_LeavesADraftMarkThatAlreadySaysIt(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	pushWrites(t, s, p.ID, "", []*model.Block{blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated)}, nil)
	_, err := s.UpsertUnitDecisions(t.Context(), p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status: string(model.TargetStatusEstablished), ReviewState: venue.ReviewStateApproved,
		Revision: nbTextRevision("Hei"), Basis: sourceRevision("Hello"),
		DecidedBy: "u-reviewer", Updated: "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)
	w := venue.EditionWrite{ItemName: "en.json", Unit: "greeting", Variant: "nb", Revision: nbRevision(t, s, p.ID, "greeting"),
		Basis: sourceRevision("Hello"), Writer: venue.WriterTool, Origin: "flow:up"}
	updatedAt := func() string {
		var at string
		require.NoError(t, s.db.QueryRowContext(t.Context(),
			`SELECT updated_at::text FROM unit_decisions WHERE project_id=$1 AND unit='greeting' AND variant='nb'`, p.ID).Scan(&at))
		return at
	}
	pushWrites(t, s, p.ID, "u-ada", nil, []venue.EditionWrite{w})
	drafts, err := s.ListDraftBases(t.Context(), p.ID, "main")
	require.NoError(t, err)
	require.Len(t, drafts, 1, "the run's draft is marked on the decided unit's row")
	first := updatedAt()
	pushWrites(t, s, p.ID, "u-ada", nil, []venue.EditionWrite{w})
	assert.Equal(t, first, updatedAt(), "the second push wrote nothing to the record")
}
