package source

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	coreproj "github.com/neokapi/neokapi/core/project"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/venue"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	"github.com/neokapi/neokapi/host"
	bproject "github.com/neokapi/neokapi/host/venue/project"
)

// The context content type's pull half.
//
// A pull carries every collection the server holds, each stating its owner, and
// this is where that field earns its place. A workspace-owned collection is the
// server's — the pull records it and that is the whole of it, because there is
// no local governance for it to conflict with. A recipe-owned collection is
// git's: the pull records what the server holds, compares it with what the
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
	// Differences are the voice profiles whose server version differs from
	// the local one, with what the pull did about each.
	Differences []bowrainconn.RecordDifference
}

// applyPulledContext records the pulled context on the sync cache and reports
// what diverged. kapi.yaml is never touched. The local voice store is written
// only when opts.TakeServer asks for the server's version of a profile
// (settleVoice).
func (c *BowrainSourceConnector) applyPulledContext(ctx context.Context, entries []*pb.SyncContextEntry, opts bowrainconn.PullOptions) (*ContextPullResult, error) {
	result := &ContextPullResult{}
	if len(entries) == 0 {
		return result, nil
	}
	settled := map[string]bool{}

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
		if key := strings.ToLower(e.VoiceProfile); key != "" && !settled[key] {
			settled[key] = true
			if err := c.settleVoice(ctx, e, opts, result); err != nil {
				return nil, err
			}
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
	return result, nil
}

// settleVoice compares the server's version of the voice profile e binds with
// the one this project resolves, and records the server's revision as seen
// (SyncCache.VoiceSeen), which a later push names as its base, only when the
// person cannot overwrite an edit unseen by pushing:
//
//   - the two agree;
//   - opts.TakeServer, and the server's version was written locally;
//   - opts.KeepLocal: the person read the difference and means their version
//     to replace the server's.
//
// Otherwise the difference is reported, field by field, and the revision
// stays: the next push that changes the profile is refused until one of the
// above happens. That holds for a script as for a person at a terminal, so no
// run moves past someone else's edit without a flag that says so.
func (c *BowrainSourceConnector) settleVoice(ctx context.Context, e *pb.SyncContextEntry, opts bowrainconn.PullOptions, result *ContextPullResult) error {
	rev := e.GetVoiceProfileRevision()
	if rev == "" || len(e.VoiceProfileJson) == 0 {
		return nil
	}
	local, source := c.localVoice(ctx, e.Name)
	if local == nil || !strings.EqualFold(local.Name, e.VoiceProfile) {
		// No push from this project carries that profile.
		return nil
	}
	var server coreprofile.VoiceProfile
	if err := json.Unmarshal(e.VoiceProfileJson, &server); err != nil {
		return fmt.Errorf("read the server's voice profile %q: %w", e.VoiceProfile, err)
	}
	key := strings.ToLower(e.VoiceProfile)
	fields := voiceDifferences(&server, local)
	if len(fields) == 0 {
		c.seeVoice(key, rev)
		return nil
	}
	d := bowrainconn.RecordDifference{
		Kind:      "voice profile",
		Name:      server.Name,
		ChangedBy: server.UpdatedBy,
		ChangedAt: server.UpdatedAt,
		Fields:    fields,
		Outcome:   bowrainconn.DifferenceUnresolved,
	}
	switch {
	case opts.TakeServer:
		if _, err := c.app.TakeServerVoice(ctx, c.project.Layout.RecipePath, c.project.Root, source, local, &server); err != nil {
			return err
		}
		c.seeVoice(key, rev)
		d.Outcome = bowrainconn.DifferenceTaken
	case opts.KeepLocal:
		c.seeVoice(key, rev)
		d.Outcome = bowrainconn.DifferenceKept
	}
	result.Differences = append(result.Differences, d)
	return nil
}

// seeVoice records rev as the server's version of the profile this project
// has seen.
func (c *BowrainSourceConnector) seeVoice(key, rev string) {
	if c.cache.VoiceSeen == nil {
		c.cache.VoiceSeen = map[string]string{}
	}
	c.cache.VoiceSeen[key] = rev
}

// voiceDifferences lists the fields a push of local would change on server,
// each rendered for a reader: the fields the server copies from a pushed
// profile, constraints only when local states them.
func voiceDifferences(server, local *coreprofile.VoiceProfile) []bowrainconn.FieldDifference {
	type field struct {
		name          string
		server, local any
	}
	fields := []field{
		{"description", server.Description, local.Description},
		{"tone", server.Tone, local.Tone},
		{"style", server.Style, local.Style},
		{"examples", server.Examples, local.Examples},
		{"locales", server.Locales, local.Locales},
		{"channels", server.Channels, local.Channels},
		{"personas", server.Personas, local.Personas},
		{"min_score", server.MinScore, local.MinScore},
	}
	if local.Constraints != nil {
		fields = append(fields, field{"constraints", server.Constraints, local.Constraints})
	}
	var out []bowrainconn.FieldDifference
	for _, f := range fields {
		s, l := renderVoiceField(f.server), renderVoiceField(f.local)
		if s != l {
			out = append(out, bowrainconn.FieldDifference{Field: f.name, Server: s, Local: l})
		}
	}
	return out
}

// renderVoiceField renders one field's value: a string as itself, anything
// else as compact JSON, and every empty value as "(none)", so an empty list
// and a missing one read, and compare, the same.
func renderVoiceField(v any) string {
	if s, ok := v.(string); ok {
		if s == "" {
			return "(none)"
		}
		return s
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	switch string(raw) {
	case "null", "[]", "{}", `""`, "0":
		return "(none)"
	}
	return string(raw)
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
	recipe := c.project.Recipe
	governance, err := recipe.ResolveGovernance(e.Name)
	if err != nil {
		return nil
	}
	var parts []string
	// The same point the push carries (CoordinatesAt), or every collection
	// with a declared axis would read as diverged forever: the entry on the
	// server holds the merged point.
	point := recipe.CoordinatesAt(coreproj.GovernancePoint{Collection: e.Name}, governance)
	if !maps.Equal(normalizeCoordinates(point), normalizeCoordinates(e.Coordinates)) {
		parts = append(parts, "point")
	}
	if governance.Channel != e.Channel {
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

// localVoice is the voice profile this project resolves for collection, as
// a push would carry it, and where it comes from ("store:<id>",
// "pack:<name>"). A profile that cannot be resolved is nil.
func (c *BowrainSourceConnector) localVoice(ctx context.Context, collection string) (*coreprofile.VoiceProfile, string) {
	if c.app == nil || c.project == nil || c.project.Recipe == nil {
		return nil, ""
	}
	store, release, serr := c.app.ProjectVoiceStore(ctx, c.project.Root)
	if serr != nil {
		return nil, ""
	}
	defer release()
	profile, _, source, found, err := c.app.LoadCollectionVoice(ctx, &c.project.Recipe.KapiProject, c.project.Root,
		host.VoiceResolveOptions{
			Point: c.app.GovernancePointFor(collection, ""),
			Store: store,
		})
	if err != nil || !found {
		return nil, ""
	}
	return profile, source
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
