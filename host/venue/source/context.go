package source

import (
	"context"
	"maps"
	"slices"
	"strings"

	coreproj "github.com/neokapi/neokapi/core/project"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/host"
	bproject "github.com/neokapi/neokapi/host/venue/project"
)

// The context content type's pull half.
//
// A pull carries every collection the server holds, each stating its owner, and
// this is where that field earns its place. A workspace-owned collection is the
// server's — the pull records it and that is the whole of it, because there is
// no local governance for it to conflict with. A recipe-owned collection is
// git's: the pull records what the server thinks, compares it with what the
// recipe says, and REPORTS a difference rather than resolving one.
//
// The asymmetry is the point. Governance that could be rewritten by a pull is
// governance nobody can rely on: the same content would resolve to a different
// voice depending on where the loop last ran. So the local resolution path
// (core/project.ResolveGovernance over kapi.yaml) is never fed from here — this
// file writes only into the sync cache, which is observation, and the one thing
// it produces for a caller is a list of divergences to print.

// ContextPullResult reports what a pull's context entries amounted to.
type ContextPullResult struct {
	// Observed is how many collections the server reported.
	Observed int
	// WorkspaceOwned names the collections the server governs. The recipe
	// declares none of these, so nothing local was overwritten by recording
	// them.
	WorkspaceOwned []string
	// Diverged names recipe-owned collections whose server-side governance
	// differs from what the recipe resolves. Reported, never applied.
	Diverged []string
	// DivergedDetail says WHICH part of the governance differs, per diverged
	// collection ("docs: voice"). A name alone leaves the reader to diff two
	// sides they cannot see — the point (coordinates), the channel and the
	// voice are three different lines to reconcile in kapi.yaml.
	DivergedDetail []string
}

// applyPulledContext records the pulled context on the sync cache and reports
// what diverged. It writes nothing outside the cache: kapi.yaml is not touched,
// the local brand store is not touched, and the profile a run resolves is
// exactly the one the recipe binds — before this call and after it.
func (c *BowrainSourceConnector) applyPulledContext(ctx context.Context, entries []*pb.SyncContextEntry) *ContextPullResult {
	result := &ContextPullResult{}
	if len(entries) == 0 {
		return result
	}

	observed := make(map[string]bproject.ServerCollection, len(entries))
	for _, e := range entries {
		if e == nil || e.Name == "" {
			continue
		}
		owner := venue.NormalizeContextOwner(e.Owner)
		observed[e.Name] = bproject.ServerCollection{
			Coordinates:  maps.Clone(e.Coordinates),
			Channel:      e.Channel,
			VoiceProfile: e.VoiceProfile,
			Owner:        owner,
		}
		result.Observed++

		if !venue.IsRecipeOwned(owner) {
			result.WorkspaceOwned = append(result.WorkspaceOwned, e.Name)
			continue
		}
		if parts := c.recipeDivergesFrom(ctx, e); len(parts) > 0 {
			result.Diverged = append(result.Diverged, e.Name)
			result.DivergedDetail = append(result.DivergedDetail, e.Name+": "+strings.Join(parts, ", "))
		}
	}

	c.cache.ServerContext = observed
	slices.Sort(result.WorkspaceOwned)
	slices.Sort(result.Diverged)
	slices.Sort(result.DivergedDetail)
	return result
}

// recipeDivergesFrom names the parts of this collection's governance the recipe
// resolves differently from the server: its point, its channel, its voice. An
// empty result is agreement.
//
// A collection the recipe does not declare at all is not a divergence: the
// server has it marked recipe-owned because some earlier push declared it, and
// the push path already reports that as an undeclared collection. Saying it
// twice, in two vocabularies, helps nobody.
func (c *BowrainSourceConnector) recipeDivergesFrom(ctx context.Context, e *pb.SyncContextEntry) []string {
	declared := c.declaredCollection(e.Name)
	if declared == nil {
		return nil
	}
	ref, err := c.project.Recipe.ResolveChannel(declared.Channel)
	if err != nil {
		return nil
	}
	var parts []string
	// The same merge the push carries, or every collection with a declared axis
	// would read as diverged forever: the entry on the server holds the merged
	// point and this compared it against the structural axes alone.
	point := coreproj.MergeCoordinates(
		c.project.Recipe.Defaults.Coordinates,
		ref.Coordinates(),
		declared.Coordinates,
	)
	if !maps.Equal(normalizeCoordinates(point), normalizeCoordinates(e.Coordinates)) {
		parts = append(parts, "point")
	}
	if ref.Channel != e.Channel {
		parts = append(parts, "channel")
	}
	if c.voiceDivergesFrom(ctx, e.Name, e.VoiceProfile) {
		parts = append(parts, "voice")
	}
	return parts
}

// voiceDivergesFrom reports whether the voice the recipe binds to this
// collection is the one the server governs it by.
//
// The comparison is by profile NAME, which is the identity both sides speak:
// the push carries the resolved profile's name, and a pull renders the bound
// profile id back into its name for exactly this reason. Comparing the recipe's
// *binding* — a file, a starter pack, a local store entry — against a hub name
// is what would report a divergence on every pull, so the binding is resolved
// first and only the names meet.
//
// Reported, never resolved: a file-bound voice is git's, and a pull that
// rewrote it would make the same content read differently depending on where
// the loop last ran. A resolution failure makes no claim at all — an
// unreadable voice store is a fault to fix, not a divergence to announce.
func (c *BowrainSourceConnector) voiceDivergesFrom(ctx context.Context, collection, serverProfile string) bool {
	if c.app == nil || c.project == nil || c.project.Recipe == nil {
		return false
	}
	store, release, serr := c.app.ProjectVoiceStore(ctx, c.project.Root)
	if serr != nil {
		return false
	}
	defer release()
	profile, _, _, found, err := c.app.LoadCollectionVoice(ctx, &c.project.Recipe.KapiProject, c.project.Root,
		host.VoiceResolveOptions{
			Point: c.app.GovernancePointFor(collection, ""),
			Store: store,
		})
	if err != nil {
		return false
	}
	local := ""
	if found && profile != nil {
		local = profile.Name
	}
	return local != serverProfile
}

// declaredCollection finds the recipe's collection of that name, or nil.
func (c *BowrainSourceConnector) declaredCollection(name string) *coreproj.Collection {
	if c.project == nil || c.project.Recipe == nil {
		return nil
	}
	for i := range c.project.Recipe.Collections {
		if coll := &c.project.Recipe.Collections[i]; coll.Name == name {
			return coll
		}
	}
	return nil
}

// normalizeCoordinates treats a nil and an empty coordinate map as the same
// thing, so a collection that declares no point does not read as diverging from
// one the server stored as an empty map.
func normalizeCoordinates(m map[string]string) map[string]string {
	if len(m) == 0 {
		return map[string]string{}
	}
	return m
}
