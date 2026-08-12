package blockstore_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bwblockstore "github.com/neokapi/neokapi/bowrain/store/blockstore"
	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
)

// An annotation written through the block store's overlay API and one written
// through the block round-trip are the same annotation: one row, one key, one
// payload envelope. These fixtures drive the whole verb on both backends — a
// plugin's findings are asked for from every reader that legitimately holds
// them, the editor's note is asked for back through the overlay API, and the
// deletion of a block is asked to take its overlays with it.

// overlayRows counts rows in one overlay table that belong to a block under
// EITHER name it answers to — its own `blocks.id` or its content key. Deletion
// clears by the row id alone, so counting only that key would score a leak as a
// clean sweep: a row filed under the content key is invisible to the delete and
// to a probe that spells the block the same way the delete does.
func overlayRows(t *testing.T, b backend, table string, block *platstore.StoredBlock) int {
	t.Helper()
	mark := func(n int) string {
		if b.dialect == bwblockstore.PostgresDialect {
			return fmt.Sprintf("$%d", n)
		}
		return "?"
	}
	q := `SELECT count(*) FROM ` + table + ` WHERE project_id = ` + mark(1) +
		` AND block_id IN (` + mark(2) + `, ` + mark(3) + `)`
	var n int
	require.NoError(t, b.db.QueryRowContext(context.Background(), q,
		b.projectID, block.ID, block.ContentHash).Scan(&n))
	return n
}

// writeEveryOverlayKind commits one overlay of each routed kind against a block
// through the Store API, and returns the payloads it wrote.
func writeEveryOverlayKind(t *testing.T, b backend, blockKey string) map[string]string {
	t.Helper()
	payloads := map[string]string{
		"targets/fr":          `{"runs":[{"text":"Bonjour"}]}`,
		"annotations/note":    `{"items":[{"text":"check the tone","from":"reviewer"}]}`,
		"annotations/qa":      `{"findings":[{"rule":"punctuation","severity":"info"}],"score":91}`,
		"plugins/vendor.lint": `{"ruleId":"X1","hits":3}`,
	}
	sess, err := b.blocks(t).Begin(context.Background())
	require.NoError(t, err)
	for kind, payload := range payloads {
		require.NoErrorf(t, sess.PutOverlay(blockstore.Overlay{
			Kind:      kind,
			BlockHash: blockKey,
			Payload:   []byte(payload),
		}), "put %s", kind)
	}
	require.NoError(t, sess.Commit())
	return payloads
}

// TestAnnotationWrite_EveryReaderSeesTheAnnotation drives the forward leg: a
// pass commits an `annotations/<name>` overlay through the Store API, and each
// reader that legitimately holds it is asked what it sees.
//
// Before the write and the read agreed on one row, all of these failed: the
// write landed under the caller's content key with the full `annotations/<name>`
// kind and the caller's body unwrapped, while hydration reads `blocks.id`, the
// bare key and a type-discriminated envelope. The block came back with no
// annotation at all.
func TestAnnotationWrite_EveryReaderSeesTheAnnotation(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		block := seedItemBlock(t, b, "greetings.json", "tu1", "Hello")
		payloads := writeEveryOverlayKind(t, b, block.ContentHash)

		t.Run("block hydration types a registered annotation", func(t *testing.T) {
			rows, err := b.content.GetBlocks(ctx, platstore.BlockQuery{
				ProjectID: b.projectID, Stream: "main", IDs: []string{block.ID},
			})
			require.NoError(t, err)
			require.Len(t, rows, 1)
			ann, ok := rows[0].Block.Anno(model.AnnoNote)
			require.True(t, ok, "the block came back with no note annotation")
			notes, isNotes := ann.(*model.Notes)
			require.True(t, isNotes, "note annotation came back as %T", ann)
			require.Len(t, notes.Items, 1)
			require.Equal(t, "check the tone", notes.Items[0].Text)
			require.Equal(t, "reviewer", notes.Items[0].From)
		})

		t.Run("block hydration keeps a plugin schema verbatim", func(t *testing.T) {
			rows, err := b.content.GetBlocks(ctx, platstore.BlockQuery{
				ProjectID: b.projectID, Stream: "main", IDs: []string{block.ID},
			})
			require.NoError(t, err)
			require.Len(t, rows, 1)
			ann, ok := rows[0].Block.Anno("qa")
			require.True(t, ok, "the block came back with no qa annotation")
			raw, isRaw := ann.(*model.RawAnnotation)
			require.True(t, isRaw, "unregistered annotation came back as %T", ann)
			require.Equal(t, "qa", raw.TypeName())
			require.JSONEq(t, payloads["annotations/qa"], string(raw.Body))
		})

		t.Run("overlay read", func(t *testing.T) {
			sess, err := b.blocks(t).Begin(ctx)
			require.NoError(t, err)
			defer sess.Close()
			for kind, want := range payloads {
				if kind == "targets/fr" {
					continue // the target codec adds its flattened text; pinned elsewhere
				}
				got, err := sess.GetOverlay(kind, block.ContentHash)
				require.NoErrorf(t, err, "get %s", kind)
				require.Equal(t, kind, got.Kind)
				require.Equal(t, block.ContentHash, got.BlockHash)
				require.JSONEqf(t, want, string(got.Payload), "payload of %s", kind)
				require.NotZerof(t, got.UpdatedAt, "UpdatedAt of %s", kind)
			}
		})

		t.Run("overlay listing names the block the iterator names", func(t *testing.T) {
			sess, err := b.blocks(t).Begin(ctx)
			require.NoError(t, err)
			defer sess.Close()
			for _, kind := range []string{"annotations/qa", "plugins/vendor.lint"} {
				var listed []blockstore.Overlay
				for o, err := range sess.ListOverlays(kind) {
					require.NoError(t, err)
					listed = append(listed, o)
				}
				require.Lenf(t, listed, 1, "listing of %s", kind)
				require.Equal(t, block.ContentHash, listed[0].BlockHash)
				require.JSONEq(t, payloads[kind], string(listed[0].Payload))
			}
		})
	})
}

// TestBlockWrite_AnnotationOverlayReadSeesIt drives the return leg: the editor
// (and every server path that writes a block) commits an annotation through the
// block round-trip, and the overlay API reads it back under the kind its caller
// names.
func TestBlockWrite_AnnotationOverlayReadSeesIt(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		block := seedItemBlock(t, b, "greetings.json", "tu1", "Hello there")

		edited := model.NewRunsBlock(block.ID, []model.Run{{Text: &model.TextRun{Text: "Hello there"}}})
		edited.Translatable = true
		edited.SetAnno(model.AnnoNote, &model.Notes{Items: []*model.NoteAnnotation{
			{Text: "keep the exclamation", From: "reviewer", Annotates: "source"},
		}})
		require.NoError(t, b.content.StoreBlocksForItem(ctx, b.projectID, "main", "greetings.json", []*model.Block{edited}))

		sess, err := b.blocks(t).Begin(ctx)
		require.NoError(t, err)
		defer sess.Close()

		got, err := sess.GetOverlay("annotations/note", block.ContentHash)
		require.NoError(t, err, "the editor's annotation is invisible to the overlay read")
		require.JSONEq(t,
			`{"items":[{"text":"keep the exclamation","from":"reviewer","annotates":"source"}]}`,
			string(got.Payload))

		var listed []blockstore.Overlay
		for o, err := range sess.ListOverlays("annotations/note") {
			require.NoError(t, err)
			listed = append(listed, o)
		}
		require.Len(t, listed, 1)
		require.Equal(t, block.ContentHash, listed[0].BlockHash)
		require.JSONEq(t, string(got.Payload), string(listed[0].Payload))
	})
}

// TestBlockDeletion_ClearsOverlaysWrittenThroughTheAdapter pins the leak the key
// divergence caused: block deletion clears overlays by `blocks.id`, so a row
// filed under the caller's content key matched nothing and outlived the block it
// described — forever, and resurrected onto new source text if the id was ever
// re-pushed.
func TestBlockDeletion_ClearsOverlaysWrittenThroughTheAdapter(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		block := seedItemBlock(t, b, "greetings.json", "tu1", "Hello")
		writeEveryOverlayKind(t, b, block.ContentHash)

		for _, table := range []string{"translations", "annotations", "overlays_ext"} {
			require.NotZerof(t, overlayRows(t, b, table, block), "nothing written to %s", table)
		}

		require.NoError(t, b.content.DeleteBlock(ctx, b.projectID, "main", block.ID))

		for _, table := range []string{"translations", "annotations", "overlays_ext"} {
			require.Zerof(t, overlayRows(t, b, table, block),
				"%s row outlived the block it describes", table)
		}
	})
}

// TestItemDeletion_ClearsOverlaysWrittenThroughTheAdapter is the same leak on
// the other lifecycle path: deleting the document takes its blocks' overlays
// with it, whichever door they were written through.
func TestItemDeletion_ClearsOverlaysWrittenThroughTheAdapter(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		require.NoError(t, b.content.StoreItem(ctx, b.projectID, "main", &platstore.Item{Name: "greetings.json"}))
		block := seedItemBlock(t, b, "greetings.json", "tu1", "Hello")
		writeEveryOverlayKind(t, b, block.ContentHash)

		require.NoError(t, b.content.DeleteItem(ctx, b.projectID, "main", "greetings.json"))

		for _, table := range []string{"translations", "annotations", "overlays_ext"} {
			require.Zerof(t, overlayRows(t, b, table, block),
				"%s row outlived the item it belonged to", table)
		}
	})
}

// TestAnnotationOverlay_KindNamesNoAnnotation pins the other refusal: a bare
// `annotations` kind addresses no annotation key, and a row under the empty key
// is one nothing would ever look for.
func TestAnnotationOverlay_KindNamesNoAnnotation(t *testing.T) {
	ctx := context.Background()
	bs, cs, projectID := newTestStore(t)
	block := seedBlock(t, cs, projectID, "blk-bare", "Hello")

	sess, err := bs.Begin(ctx)
	require.NoError(t, err)
	defer sess.Close()

	err = sess.PutOverlay(blockstore.Overlay{
		Kind:      "annotations",
		BlockHash: block.ContentHash,
		Payload:   []byte(`{"findings":[]}`),
	})
	require.ErrorContains(t, err, "names no annotation")

	_, err = sess.GetOverlay("annotations", block.ContentHash)
	require.ErrorContains(t, err, "names no annotation")
}
