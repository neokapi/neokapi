package connector

import (
	"maps"
	"slices"

	pb "github.com/neokapi/neokapi/bowrain/core/proto/sync/v1"
	bproject "github.com/neokapi/neokapi/bowrain/core/project"
	bowsync "github.com/neokapi/neokapi/bowrain/core/sync"
	coreproj "github.com/neokapi/neokapi/core/project"
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
}

// applyPulledContext records the pulled context on the sync cache and reports
// what diverged. It writes nothing outside the cache: kapi.yaml is not touched,
// the local brand store is not touched, and the profile a run resolves is
// exactly the one the recipe binds — before this call and after it.
func (c *BowrainSourceConnector) applyPulledContext(entries []*pb.SyncContextEntry) *ContextPullResult {
	result := &ContextPullResult{}
	if len(entries) == 0 {
		return result
	}

	observed := make(map[string]bproject.ServerCollection, len(entries))
	for _, e := range entries {
		if e == nil || e.Name == "" {
			continue
		}
		owner := bowsync.NormalizeContextOwner(e.Owner)
		observed[e.Name] = bproject.ServerCollection{
			Coordinates:  maps.Clone(e.Coordinates),
			Channel:      e.Channel,
			VoiceProfile: e.VoiceProfile,
			Owner:        owner,
		}
		result.Observed++

		if !bowsync.IsRecipeOwned(owner) {
			result.WorkspaceOwned = append(result.WorkspaceOwned, e.Name)
			continue
		}
		if c.recipeDivergesFrom(e) {
			result.Diverged = append(result.Diverged, e.Name)
		}
	}

	c.cache.ServerContext = observed
	slices.Sort(result.WorkspaceOwned)
	slices.Sort(result.Diverged)
	return result
}

// recipeDivergesFrom reports whether the recipe resolves this collection to a
// different point or a different channel than the server holds.
//
// The voice profile is deliberately not compared. The server carries the name
// of a profile in its brand hub, while the recipe binds a file, a starter pack
// or a local store entry — comparing those would report a divergence on every
// pull of a project whose voice is a file, which is most of them.
//
// A collection the recipe does not declare at all is not a divergence either:
// the server has it marked recipe-owned because some earlier push declared it,
// and the push path already reports that as an undeclared collection. Saying it
// twice, in two vocabularies, helps nobody.
func (c *BowrainSourceConnector) recipeDivergesFrom(e *pb.SyncContextEntry) bool {
	declared := c.declaredCollection(e.Name)
	if declared == nil {
		return false
	}
	if !maps.Equal(normalizeCoordinates(declared.Context), normalizeCoordinates(e.Coordinates)) {
		return true
	}
	return declared.Context[coreproj.ChannelAxis] != e.Channel
}

// declaredCollection finds the recipe's collection of that name, or nil.
func (c *BowrainSourceConnector) declaredCollection(name string) *coreproj.ContentCollection {
	if c.project == nil || c.project.Recipe == nil {
		return nil
	}
	for i := range c.project.Recipe.Content {
		if coll := &c.project.Recipe.Content[i]; coll.Name == name {
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
