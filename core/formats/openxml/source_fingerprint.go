// The source runs a block was read with, as a digest the writer can check
// before it puts source bytes back in place of a rendering.

package openxml

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/neokapi/neokapi/core/model"
)

// sourceFingerprintAnnotationKey is the block annotation the reader stamps
// with a digest of the source runs it emitted. A tool that edits the source
// runs in place, the way `kapi sed` does with no target locale, leaves a
// block whose source no longer says what the reader read; the writer must
// render such a block rather than replay the bytes the reader saw, and the
// block itself is the only place that can carry the evidence.
const sourceFingerprintAnnotationKey = "openxml-source-fingerprint"

// stampSourceFingerprint records the digest of a block's source runs as the
// reader emitted them. Runs that cannot be encoded leave no stamp, and a block
// without one is treated as edited.
func stampSourceFingerprint(b *model.Block) {
	digest, ok := runsFingerprint(b.Source)
	if !ok {
		return
	}
	b.SetAnno(sourceFingerprintAnnotationKey, &model.GenericAnnotation{
		Kind:   sourceFingerprintAnnotationKey,
		Fields: map[string]any{"sha256": digest},
	})
}

// sourceRunsAsRead reports whether a block's source runs are the ones the
// reader emitted. A block with no stamp, or whose runs no longer encode to the
// stamped digest, reports false, and nothing is replayed over it.
func sourceRunsAsRead(b *model.Block) bool {
	if b == nil {
		return false
	}
	a, ok := b.Anno(sourceFingerprintAnnotationKey)
	if !ok {
		return false
	}
	g, ok := a.(*model.GenericAnnotation)
	if !ok {
		return false
	}
	want, _ := g.Fields["sha256"].(string)
	if want == "" {
		return false
	}
	got, ok := runsFingerprint(b.Source)
	return ok && got == want
}

// runsFingerprint digests a run sequence through its JSON encoding, which is
// the canonical form the model already commits to: every field of every run
// kind, in a fixed order, with map keys sorted.
func runsFingerprint(runs []model.Run) (string, bool) {
	data, err := json.Marshal(runs)
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), true
}
