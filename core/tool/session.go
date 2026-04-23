package tool

import (
	"context"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
)

// SessionTool is an optional interface tools can implement when they
// need random access to the project's block state instead of — or
// alongside — the streaming channel contract.
//
// Why this exists: the channel-based `Tool.Process` is a one-shot
// forward-only transform. Tools that need to look up a block by
// hash, read prior overlays (TM matches, QA findings, prior target
// translations), or incrementally skip work that's already done
// can't do it through the stream alone. SessionTool gives them a
// `blockstore.Session` opened by the executor on whichever provider
// the project declares (`memory`, `cache`, remote).
//
// Lifecycle:
//
//  1. Executor opens a Session against the project's declared store
//     at flow start.
//  2. For each tool in the flow, executor calls either
//     SessionProcess (if the tool implements SessionTool) or the
//     streaming Process (legacy path). Both paths can run for a
//     single tool — SessionTool.SessionProcess is allowed to also
//     iterate the stream via the in/out channels for hybrid use.
//  3. Executor commits the session if every tool succeeded, or
//     rolls back on error.
//
// Tools that declare SessionTool MUST also keep implementing the
// original Tool interface — the streaming contract is the lowest
// common denominator, and flow composition (chaining tools) assumes
// every step speaks it.
type SessionTool interface {
	Tool

	// SessionProcess runs the tool with a session handle in addition
	// to the streaming channels. Implementations can:
	//   - Ignore the session entirely (falls back to Tool.Process
	//     behaviour) — but then SessionTool isn't the right contract.
	//   - Use the session exclusively (reading blocks via
	//     session.Blocks instead of `in`, writing overlays via
	//     session.PutOverlay instead of emitting new Parts).
	//   - Mix both (read from in, enrich via session, emit to out).
	//
	// Do NOT call Commit/Rollback on the session — the executor
	// owns transaction boundaries. Calling them will return
	// ErrClosed on subsequent use.
	SessionProcess(
		ctx context.Context,
		sess blockstore.Session,
		in <-chan *model.Part,
		out chan<- *model.Part,
	) error
}
