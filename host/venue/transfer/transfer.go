// Package transfer moves a project's content between the checkout and a venue.
//
// It is orchestration rather than transport: a push composes the source
// connector, the declared context and the resolved governance into one
// consistent upload, and a pull applies what comes back. The transport itself
// lives in host/venue/client, and walking the checkout lives in
// host/venue/source; keeping the composition out of both is what lets one
// implementation serve every route a transfer arrives by.
//
// There are three such routes, and they must not diverge: the `push` and `pull`
// verbs a person types, the venue's `up` plumbing (which is a push, a server-run
// and a pull), and the automation that runs the same pair on a schedule. A
// second copy of this logic is how a push ends up carrying content while
// leaving the collections it belongs to behind.
package transfer

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/core/venue/connector"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/venue/project"
	"github.com/neokapi/neokapi/host/venue/source"
)

// PushOptions selects what a push sends and how.
type PushOptions struct {
	// Paths narrows the push to these files; empty pushes everything the
	// project tracks.
	Paths []string
	// Force pushes every block, ignoring the sync cache.
	Force bool
	// DryRun resolves and reports without sending.
	DryRun bool
	// Stream overrides the stream the connector would otherwise resolve from
	// config or auto-detection. Empty leaves that resolution alone.
	Stream string
}

// PushResult holds the structured result of a push.
type PushResult struct {
	BlocksPushed   int
	BlocksUploaded int
	WordCount      int
	FilesScanned   int
	PushID         string
	DryRun         bool
	UpToDate       bool

	// AssetsPushed/AssetsFailed/AssetErrors carry the media half of the push.
	// The failures are carried, not folded into the success count, because an
	// asset that never arrived is content the reader will not see.
	AssetsPushed int
	AssetsFailed int
	AssetErrors  []string

	// Ingest is what the server did with the upload: applied, queued, or
	// unknown. A failed ingest never reaches here — it is an error from the
	// connector's Push, which also leaves the sync cache unwritten.
	Ingest string

	// Brand reports what the governance carried in the push amounted to. Nil
	// when the project binds no voice.
	Brand *PushVoiceResult

	// UndeclaredCollections names recipe-owned collections the server holds
	// that this push no longer declares. Reported, never deleted.
	UndeclaredCollections []string

	// Governance is what the venue's review gate did not accept, and
	// VerdictsRetired how many of the project's own records were brought
	// into line with that answer.
	Governance      *venue.PushGovernance
	VerdictsRetired int
}

// PullResult holds the structured result of a pull.
type PullResult struct {
	BlocksPulled    int
	DecisionsStaged int
	LocalesCount    int
	FilesWritten    int
	ItemsRetired    int
	DryRun          bool
	UpToDate        bool

	// CollectionsObserved and GovernanceDiverged carry the context content
	// type's pull half: how many collections the server reported, and which
	// recipe-owned ones it governs differently, with the differing part named.
	// Observed, never applied.
	CollectionsObserved  int
	GovernanceDiverged   []string
	GovernanceDivergence []string
}

// Push uploads the project's changed content to its venue and returns both the
// result and the open connector, so a caller that continues the exchange — `up`
// pulls straight back — reuses the same connection rather than opening a
// second one. The caller closes it.
//
// The context content type is built here rather than at each call site, so
// every route carries the project's declared structure and governance by
// construction: there is no push path that moves content while leaving the
// collections it belongs to behind.
func Push(ctx context.Context, app *host.App, opts PushOptions) (*PushResult, *source.BowrainSourceConnector, error) {
	proj, err := project.FindProject("")
	if err != nil {
		return nil, nil, err
	}

	conn, err := source.NewSourceConnector(app, proj, app.FormatReg)
	if err != nil {
		return nil, nil, err
	}
	if opts.Stream != "" {
		conn.SetStream(opts.Stream)
	}

	pushCtx, brand, err := BuildPushContext(ctx, app, proj, opts.DryRun)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	conn.SetPushContext(pushCtx)

	result, err := conn.Push(ctx, connector.PushOptions{
		Paths:  opts.Paths,
		Force:  opts.Force,
		DryRun: opts.DryRun,
	})
	if err != nil {
		conn.Close()
		return nil, nil, err
	}

	pr := &PushResult{
		BlocksPushed:          result.BlocksPushed,
		BlocksUploaded:        result.BlocksUploaded,
		WordCount:             result.WordCount,
		FilesScanned:          result.FilesScanned,
		PushID:                result.PushID,
		Brand:                 brand,
		UndeclaredCollections: result.UndeclaredCollections,
		AssetsPushed:          result.AssetsPushed,
		AssetsFailed:          result.AssetsFailed,
		AssetErrors:           result.AssetErrors,
		Ingest:                result.Ingest,
		Governance:            result.Governance,
		VerdictsRetired:       result.VerdictsRetired,
	}
	if opts.DryRun {
		pr.DryRun = true
	} else if result.BlocksPushed == 0 {
		pr.UpToDate = true
	}

	return pr, conn, nil
}

// Pull downloads translations and governed terminology into the checkout.
//
// A non-nil conn is used as given — the caller owns it — so a push that has
// just finished can be pulled through without reconnecting. A nil conn opens
// one for the duration and closes it.
func Pull(ctx context.Context, app *host.App, conn *source.BowrainSourceConnector, locales []string, force, dryRun bool) (*PullResult, error) {
	if conn == nil {
		proj, err := project.FindProject("")
		if err != nil {
			return nil, err
		}
		var connErr error
		conn, connErr = source.NewSourceConnector(app, proj, app.FormatReg)
		if connErr != nil {
			return nil, connErr
		}
		defer conn.Close()
	}

	modelLocales := make([]model.LocaleID, len(locales))
	for i, l := range locales {
		modelLocales[i] = model.LocaleID(l)
	}

	result, err := conn.Pull(ctx, connector.PullOptions{
		Locales: modelLocales,
		Force:   force,
		DryRun:  dryRun,
	})
	if err != nil {
		return nil, err
	}

	pr := &PullResult{
		BlocksPulled:         result.BlocksPulled,
		DecisionsStaged:      result.DecisionsStaged,
		LocalesCount:         result.LocalesCount,
		FilesWritten:         result.FilesWritten,
		ItemsRetired:         result.ItemsRetired,
		CollectionsObserved:  result.CollectionsObserved,
		GovernanceDiverged:   result.GovernanceDiverged,
		GovernanceDivergence: result.GovernanceDivergence,
	}
	if dryRun {
		pr.DryRun = true
	} else if result.BlocksPulled == 0 && result.DecisionsStaged == 0 {
		pr.UpToDate = true
	}

	return pr, nil
}
