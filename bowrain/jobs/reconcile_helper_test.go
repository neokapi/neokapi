package jobs

import (
	"context"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"

	"github.com/neokapi/neokapi/bowrain/voicewrite"
)

// reconcileAndWriteVoice is reconcileContext followed by the held-back voice
// updates, the order a push runs them in.
func reconcileAndWriteVoice(
	ctx context.Context,
	collections collectionWriter,
	voice coreprofile.Store,
	projectID, stream, workspaceID, actorID string,
	ledger voicewrite.Ledger,
	entries []*pb.SyncContextEntry,
) (*contextReconcileResult, error) {
	res, err := reconcileContext(ctx, collections, voice, projectID, stream, workspaceID, actorID, ledger, entries)
	if err != nil {
		return res, err
	}
	return res, res.writeVoice(ctx)
}
