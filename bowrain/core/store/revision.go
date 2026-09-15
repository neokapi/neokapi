package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// ErrBlockChanged reports that a block changed after the caller read it. A
// write made against that read is refused, and the caller is answered with the
// block as it now stands.
var ErrBlockChanged = errors.New("block changed since it was read")

// TargetRevision identifies one locale's target on a block as a reader sees
// it: the block's source, and that locale's target runs and status. A single
// write names the revision it read, and lands only while the block still has it
// (see BlockStore.UpdateBlock). A locale with no target has a revision too, so
// two first translations of one block cannot overwrite each other.
func TargetRevision(sb *venue.StoredBlock, locale model.LocaleID) string {
	var (
		contentHash string
		runs        []model.Run
		status      model.TargetStatus
	)
	if sb != nil {
		contentHash = sb.ContentHash
		if sb.Block != nil {
			runs = sb.Block.TargetRuns(locale)
			if t := sb.Block.Target(locale); t != nil {
				status = t.Status
			}
		}
	}
	runsJSON, err := json.Marshal(runs)
	if err != nil {
		runsJSON = fmt.Append(nil, runs)
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s", contentHash, locale, runsJSON, status)
	return hex.EncodeToString(h.Sum(nil))[:16]
}
