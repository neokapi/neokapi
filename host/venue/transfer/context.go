package transfer

import (
	"context"
	"encoding/json"
	"fmt"

	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/host"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	bproject "github.com/neokapi/neokapi/host/venue/project"

	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
)

// contextsync.go builds the context content type a push carries: the
// collections the recipe declares, the point each occupies in the project's
// context space, and the governance resolved for that point.
//
// It replaces the separate brand-profile upload this file's predecessor
// performed. That upload was a REST call made after the content transport had
// already finished, which meant a push could — and on a 403 routinely did —
// leave content stored against governance that never landed. Carrying the
// voice inside the push makes one push one consistent state: the worker
// reconciles the collections and upserts their voices in the same job that
// stores the blocks.
//
// The upsert semantics are unchanged, because the worker applies them: matched
// by name within the workspace, a no-op when the content is identical, and
// otherwise a new version through the store's profile versioning, so a
// server-side edit is superseded by something revertible rather than clobbered.

// PushBrandResult holds what the governance half of a push amounted to.
type PushBrandResult struct {
	Name    string // profile name
	Action  string // carried | skipped | would-push (dry run)
	Version int    // stored profile version after the action (0 when unknown)
	Reason  string // set when Action == "skipped"
}

// BuildPushContext resolves the recipe's declared context into the entries a
// push carries, and reports what happened to the governance.
//
// Exported because a push arrives by two routes and both must declare the same
// context. `kapi-bowrain push` runs the cobra command below; `kapi push` is
// dispatched over the Mode-C daemon RPC and lands in the daemon's Push handler,
// which has a project and a connector but none of this command's plumbing.
// Leaving the daemon route without it is what let a push carry 21,894 blocks
// and reconcile zero collections: no context hash on the wire reads, correctly,
// as "this push makes no claim about the declared context".
//
// A dry run resolves everything and returns the entries without a caller ever
// sending them, so `--dry-run` reports the governance it would carry.
//
// Returns (nil, nil, nil) when the project is not connected to a server: there
// is no context to reconcile against, and this is not an error — the same
// silent skip the terminology push makes.
func BuildPushContext(ctx context.Context, app *host.App, proj *bproject.Project, dryRun bool) (*apiclient.PushContext, *PushBrandResult, error) {
	if app == nil || proj == nil || proj.Recipe == nil {
		return nil, nil, nil
	}

	kapiProject := &proj.Recipe.KapiProject
	store, release, err := app.ProjectVoiceStore(ctx, proj.Root)
	if err != nil {
		return nil, nil, fmt.Errorf("open the project's voice store: %w", err)
	}
	defer release()

	// Profiles are carried once per distinct name: several collections
	// governed by one voice cost one copy of it on the wire, and the entries
	// after the first identify it by name alone.
	carried := map[string]bool{}
	var entries []*pb.SyncContextEntry
	var brandResult *PushBrandResult

	for i := range kapiProject.Collections {
		coll := &kapiProject.Collections[i]
		if coll.Name == "" {
			// A bare entry declares no collection, so there is nothing to
			// reconcile: its files sync ungrouped, governed by the project's
			// default point.
			continue
		}

		// The point is resolved at the run's instant, through the same seam a
		// run and a check use, so a push carries the governance actually in
		// force: a profile outside its validity window governs nothing, and the
		// coordinates fall through with it.
		point := app.GovernancePointFor(coll.Name, "")
		profile, governance, _, found, err := app.LoadCollectionVoice(ctx, kapiProject, proj.Root, host.VoiceResolveOptions{
			Point: point,
			Store: store,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("resolve governance for collection %q: %w", coll.Name, err)
		}
		app.NoteGovernance(nil, governance)

		// The point this collection sits at: the project's declared defaults,
		// overlaid with the structural axes the resolved governance names, then
		// with whatever the collection declares for itself. Most specific wins,
		// per axis — so a project states its brand once and a collection that
		// genuinely sits elsewhere moves on that axis alone.
		entry := &pb.SyncContextEntry{
			Name: coll.Name,
			Coordinates: coreproj.MergeCoordinates(
				kapiProject.Defaults.Coordinates,
				governance.Ref().Coordinates(),
				coll.Coordinates,
			),
			Owner: venue.ContextOwnerRecipe,
		}
		if governance != nil {
			entry.Channel = governance.Channel
		}
		// Where this collection's strings can be read in place. A repository
		// fact the server cannot derive, so it travels with the collection's
		// other declared facts rather than being configured twice.
		if preview := bproject.CollectionPreview(coll); preview != nil {
			entry.Preview = &pb.SyncPreviewSource{
				Kind: string(preview.Kind),
				Url:  preview.URL,
			}
		}

		if found && brandResult == nil {
			brandResult = &PushBrandResult{Name: profile.Name, Action: brandAction(dryRun)}
		}
		if found {
			entry.VoiceProfile = profile.Name
			if !carried[profile.Name] {
				authored, merr := json.Marshal(profile)
				if merr != nil {
					return nil, nil, fmt.Errorf("encode voice profile %q: %w", profile.Name, merr)
				}
				entry.VoiceProfileJson = authored
				carried[profile.Name] = true
			}
		}
		entries = append(entries, entry)
	}

	// A recipe that declares no collections still makes a claim — the empty one
	// — which is what lets a recipe that just dropped its last collection be
	// told the server still holds it. Only a caller with no recipe at all
	// (a nil PushContext) says nothing.
	return apiclient.NewPushContext(entries), brandResult, nil
}

// brandAction names what a push does to the governance it carries. There is no
// created/updated/unchanged distinction to report here, and that is the honest
// consequence of folding the upsert into the push: the worker decides which of
// those it was, after the client has gone.
func brandAction(dryRun bool) string {
	if dryRun {
		return "would-push"
	}
	return "carried"
}
