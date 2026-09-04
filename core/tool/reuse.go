package tool

import (
	"context"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
)

// StoredTargetReuser is a producer that serves a target the project block store
// already holds instead of producing one again, and can say, for one block,
// whether it would.
//
// The decision rests on three anchors the producer computes for itself: the
// source wording, its own configuration fingerprint (engine, model, prompt, and
// whatever reference material the block's prompt carries) and the governing
// context fingerprint. Only the producer knows the middle one, because it is
// assembled from the configuration the producer was built with, so the question
// is asked of the producer rather than reconstructed beside it. A run asks it
// on every block before calling out, and `kapi up --plan` asks it of a producer
// built the way the run builds one, so the two cannot answer differently.
type StoredTargetReuser interface {
	// ReusesStoredTarget reports whether the producer would serve stored for b
	// rather than produce a new target. It reads nothing but the block's source
	// side and the producer's own configuration, and calls no provider.
	ReusesStoredTarget(ctx context.Context, b *model.Block, stored blockstore.TargetOverlay) bool
}
