package connector

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// PushOptions configures a push operation from a source system to Bowrain.
type PushOptions struct {
	Paths  []string // Specific file paths to push (empty = all)
	Force  bool     // Push all blocks, ignoring sync cache
	DryRun bool     // Report what would be pushed without sending
}

// PullOptions configures a pull operation from Bowrain to a source system.
type PullOptions struct {
	Locales []model.LocaleID // Target locales to pull (empty = all)
	Force   bool             // Overwrite local changes
	DryRun  bool             // Report what would be pulled without writing
}

// PushResult summarizes the result of a push operation.
type PushResult struct {
	BlocksPushed int
	AssetsPushed int // number of media assets pushed (Bowrain AD-007)
	FilesScanned int
	ChunkCount   int
	WordCount    int    // total source words across pushed blocks
	PushID       string // server-assigned push correlation ID (empty if nothing stored)

	// BlocksUploaded is what actually went on the wire after diff negotiation.
	// BlocksPushed counts the locally-changed candidates; on a fresh server the
	// two must agree, and a push that reports thousands of candidates and zero
	// uploads is a keying bug this field exists to expose.
	BlocksUploaded int

	// UndeclaredCollections names recipe-owned collections the server holds
	// that this push no longer declares. Reported so the push can say so, never
	// deleted — the content grouped under them is still there.
	UndeclaredCollections []string

	// AssetsFailed counts media assets this push tried and failed to upload.
	// Asset upload is best-effort per asset — one unreachable blob must not
	// abandon the content already stored — but "best-effort" is not
	// "unreported": an image that never arrived is content the reader will not
	// see, and AssetsPushed alone cannot distinguish "nothing to send" from
	// "everything was refused".
	AssetsFailed int

	// AssetErrors names the first few asset failures, so the count above points
	// at something actionable rather than only asserting that something broke.
	AssetErrors []string

	// Ingest reports what the SERVER did with this push, not what the client
	// sent it. The commit is answered with 202 Accepted and a push id: the
	// content is applied later by a worker, so a client that stops at the 202
	// has reported success for work that may still fail — and it has already
	// recorded the pushed hashes, so the next push sees nothing to send and the
	// content is stranded. One of:
	//
	//	IngestApplied   — the server's ingest job completed.
	//	IngestQueued    — accepted, not yet confirmed within the wait window.
	//	IngestUnknown   — the server could not be asked (no job system, older
	//	                  server, status call failed).
	//	""              — nothing was sent (dry run, up-to-date).
	//
	// An ingest that FAILED is never a field: it is an error from Push, and the
	// sync cache is left unwritten so the next push sends the content again.
	Ingest string

	// Governance is what the venue's review gate did not accept: approvals
	// and sign-offs this push carried that the pusher was not entitled to
	// make. The content landed regardless, at translated. Nil when the push
	// carried no verdict, or when every verdict it carried was accepted.
	Governance *venue.PushGovernance

	// VerdictsRetired counts the project's own records this push brought
	// into line with the venue's answer, so the same refused verdicts are
	// not sent again on every push from here on.
	VerdictsRetired int
}

// Ingest states reported by PushResult.Ingest.
const (
	IngestApplied = "applied"
	IngestQueued  = "queued"
	IngestUnknown = "unknown"
)

// PullResult summarizes the result of a pull operation.
type PullResult struct {
	BlocksPulled int
	FilesWritten int
	LocalesCount int

	// DecisionsStaged is how many server-ledger decisions the pull reconciled
	// into the working store. Staged, not committed — the pull reports the
	// count so an arriving decision is never invisible, and `kapi commit`
	// remains the only door into the tracked record.
	DecisionsStaged int

	// ItemsRetired is how many items the server still streams translations
	// for whose source file this checkout no longer has — deleted or renamed
	// after it was pushed (push is additive-only, so the server never
	// forgets). Nothing can be written for them, now or on any retry, so the
	// pull skips them and moves the cursor on rather than wedging the stream.
	ItemsRetired int

	// DecisionsSkipped is how many arriving decisions could not be staged
	// because their variant did not parse. Reported because the stream cursor
	// is forward-only: a decision skipped on a pull is never offered again, so
	// dropping one in silence loses a review and its attribution for good.
	DecisionsSkipped int

	// CollectionsObserved is how many collections the server reported. They are
	// recorded as observation only — a pull never rewrites the governance the
	// recipe declares.
	CollectionsObserved int

	// GovernanceDiverged names recipe-owned collections the server governs
	// differently from the recipe. Reported so the divergence is visible; never
	// resolved here, because kapi.yaml is the authority.
	GovernanceDiverged []string

	// GovernanceDivergence says which part of each one differs — its point, its
	// channel, its voice — as "<collection>: <parts>". The names alone leave a
	// reader diffing two sides they cannot see.
	GovernanceDivergence []string
}

// SourceConnector represents a content source that pushes to and pulls from Bowrain.
// Used by systems outside Bowrain (kapi CLI, Git hooks, CI/CD).
// Terminology: from the source system's perspective.
type SourceConnector interface {
	ConnectorBase

	// Push sends source content FROM the source system TO Bowrain.
	Push(ctx context.Context, opts PushOptions) (*PushResult, error)

	// Pull retrieves translated content FROM Bowrain TO the source system.
	Pull(ctx context.Context, opts PullOptions) (*PullResult, error)
}
