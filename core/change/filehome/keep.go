package filehome

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// An edition with no file yet, such as the drafts of a locale its recipe
// withholds until it clears its ship gate, is kept by a Keeper: the workspace
// home (core/workhome). Its EditionFile names the file a delivery will write
// it to, and Kept names the keeper that holds its text until then. The file
// home joins the kept edition to the document by block key, as it joins an
// edition file, stages a change to it beside the document's own files, and
// commits it through the keeper, whose commit appends the change's record
// only while the edition's head is still the one the stage read. The
// commit lock is the lock of the file the edition will be delivered to, so a
// delivery and a write to the kept edition take turns.

// Keeper keeps the editions of documents that have no file yet.
type Keeper interface {
	// Name is the home as a result reports it.
	Name() string
	// Edition reads what the keeper holds of edition k of the document ref
	// names.
	Edition(ctx context.Context, ref string, k model.EditionKey) (Kept, error)
	// Commit stores w, only while the edition's head is still the one w.Token
	// names, and returns the id of the record it appended. A head that moved
	// is an *change.Error with CodeDocChanged, and nothing is stored.
	Commit(ctx context.Context, w KeptWrite) (string, error)
}

// Kept is what a keeper holds of one edition.
type Kept struct {
	// Token names the head the edition was read at, which a commit must
	// still find.
	Token string
	// Digest is the digest of the edition as held, "" when nothing is held.
	Digest string
	// Blocks holds each block's edition, by the document's block key.
	Blocks map[string]model.Edition
}

// KeptWrite is a change to one edition a keeper holds.
type KeptWrite struct {
	// Doc is the document's reference; File the file the edition will be
	// delivered to.
	Doc     string
	File    string
	Edition model.EditionKey
	// Token is Kept.Token as the stage read it.
	Token string
	// Before and After are the edition's digests around the change.
	Before string
	After  string
	// Changes are the blocks the change gives the edition, in document order.
	Changes []KeptChange
	// Record is the record of the change the service hands the commit
	// (change.RecordingStaged); nil when the commit was not given one.
	Record *change.Record
}

// KeptChange is one block's edition as a change leaves it.
type KeptChange struct {
	// Block is the document's block key, and Before the edition's revision
	// as the stage read it (model.AbsentRevision for an edition the change
	// creates).
	Block  string
	Before string
	// Edition is the edition the change leaves; nil removes it.
	Edition *model.Edition
}

// keptPart is the share of a stage a keeper commits: one kept edition.
type keptPart struct {
	keeper Keeper
	write  KeptWrite
}

// keptBlocks are the blocks of a kept edition, as a join pairs them: one per
// block the keeper holds, keyed by the document's block key, holding the
// edition as its own content.
func keptBlocks(k Kept) []*model.Block {
	keys := slices.Sorted(maps.Keys(k.Blocks))
	out := make([]*model.Block, 0, len(keys))
	for _, key := range keys {
		b := &model.Block{Key: key, Translatable: true}
		b.SetEdition(model.EditionKey{}, k.Blocks[key])
		out = append(out, b)
	}
	return out
}

// KeptEntry is what a kept edition's digest covers of one block's edition:
// its revision, its status and its origin, which a keeper holds with the
// runs.
func KeptEntry(k model.EditionKey, ed model.Edition) string {
	return KeptEntryOf(model.RunsRevision(k, ed.Runs), ed.Status, ed.Origin)
}

// KeptEntryOf is KeptEntry for an edition held as its revision, status and
// origin.
func KeptEntryOf(rev string, status model.Status, origin model.Origin) string {
	o, _ := json.Marshal(origin)
	return rev + "\x00" + string(status) + "\x00" + string(o)
}

// KeptDigest is the digest of a kept edition: over each block's KeptEntry,
// in block key order. It is "" for an edition with no block.
func KeptDigest(k model.EditionKey, blocks map[string]model.Edition) string {
	entries := make(map[string]string, len(blocks))
	for key, ed := range blocks {
		entries[key] = KeptEntry(k, ed)
	}
	return KeptDigestOf(entries)
}

// KeptDigestOf is KeptDigest over the blocks' entries (KeptEntryOf).
func KeptDigestOf(entries map[string]string) string {
	if len(entries) == 0 {
		return ""
	}
	h := sha256.New()
	for _, key := range slices.Sorted(maps.Keys(entries)) {
		h.Write([]byte(key))
		h.Write([]byte{0})
		h.Write([]byte(entries[key]))
		h.Write([]byte{0})
	}
	return "ws:" + hex.EncodeToString(h.Sum(nil))
}

// keptDiff renders the change a kept edition stages, for a person reading a
// preview.
func keptDiff(ref string, k model.EditionKey, before map[string]model.Edition, changes []KeptChange) string {
	if len(changes) == 0 {
		return ""
	}
	key, _ := k.MarshalText()
	var b strings.Builder
	fmt.Fprintf(&b, "--- %s (workspace)\n+++ %s (workspace)\n", ref, ref)
	for _, c := range changes {
		fmt.Fprintf(&b, "@@ %s %s @@\n", c.Block, key)
		if was, ok := before[c.Block]; ok {
			fmt.Fprintf(&b, "-%s\n", model.RunsEditText(was.Runs))
		}
		if c.Edition != nil {
			fmt.Fprintf(&b, "+%s\n", model.RunsEditText(c.Edition.Runs))
		}
	}
	return b.String()
}
