package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/id"
	coreprofile "github.com/neokapi/neokapi/core/profile"

	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/venue"
)

// The context content type's server half: turning the collections a recipe
// declares into rows the resolution path reads.
//
// What "reconcile" means here is narrow and deliberate. A declared collection
// gets a store.Collection row, and the governance resolved for its point is
// written where core/profile already looks for it — bowrain/core/brandscope
// reads Collection.ConnectorConfig into brand.ResolveContext.CollectionConfig,
// where PropertyProfileID and PropertyChannel are the keys that decide which
// voice governs the content. Nothing here invents a second place to put
// governance; it fills the one that was already being read and found empty.
//
// Two properties the tests pin:
//
//   - IDEMPOTENT. An entry whose stored context hash is unchanged is left
//     alone — not rewritten with identical values, which would churn
//     UpdatedAt and make "when did this collection last change?" unanswerable.
//   - NON-DESTRUCTIVE. A collection the recipe no longer declares is reported
//     and left standing. The push init already told the client about it; the
//     worker says so again in the log and on the completion event. Deleting it
//     would take the content grouped under it, and a recipe edit is not consent
//     to that.

// contextReconcileResult counts what a reconcile did, for the log line and the
// push-completed event.
type contextReconcileResult struct {
	Created   []string
	Updated   []string
	Claimed   []string
	Unchanged []string
	// Undeclared lists recipe-owned collections the server holds that this push
	// did not declare. Reported, never deleted.
	Undeclared []string
	// ProfileIDs maps collection name → the brand-hub profile id bound to it,
	// so the caller can see what governance actually landed.
	ProfileIDs map[string]string
}

// total reports how many collections the reconcile touched or confirmed.
func (r *contextReconcileResult) total() int {
	return len(r.Created) + len(r.Updated) + len(r.Claimed) + len(r.Unchanged)
}

// parseContextEntries decodes the manifest's context payload. An unparseable
// payload fails the push rather than storing the items into a structure that
// was only half described — the same reasoning as the item metadata above it.
func parseContextEntries(raw json.RawMessage) ([]*pb.SyncContextEntry, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var entries []*pb.SyncContextEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("parse manifest context entries: %w", err)
	}
	return entries, nil
}

// reconcileContext brings the server's collections in line with the context a
// push declared, and returns what it did.
//
// workspaceID is the workspace whose brand hub holds the voice profiles; an
// empty one (an unclaimed project) skips the profile binding and reconciles
// structure alone, which is the honest outcome — there is no hub to bind to.
func reconcileContext(
	ctx context.Context,
	deps *WorkerDeps,
	projectID, stream, workspaceID, actorID string,
	entries []*pb.SyncContextEntry,
) (*contextReconcileResult, error) {
	result := &contextReconcileResult{ProfileIDs: map[string]string{}}
	if deps.ContentStore == nil || len(entries) == 0 {
		return result, nil
	}

	existing, err := deps.ContentStore.ListCollections(ctx, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	byName := make(map[string]*store.Collection, len(existing))
	for _, col := range existing {
		if col != nil {
			byName[col.Name] = col
		}
	}

	declared := make(map[string]struct{}, len(entries))
	profileIDs := newProfileBinder(deps, workspaceID, actorID)

	for _, e := range entries {
		if e == nil || e.Name == "" {
			continue
		}
		declared[e.Name] = struct{}{}

		profileID, perr := profileIDs.bind(ctx, e)
		if perr != nil {
			return nil, perr
		}
		if profileID != "" {
			result.ProfileIDs[e.Name] = profileID
		}

		current, found := byName[e.Name]
		if !found {
			col := &store.Collection{
				ProjectID:       projectID,
				Name:            e.Name,
				Kind:            store.CollectionConnected,
				Stream:          stream,
				Context:         maps.Clone(e.Coordinates),
				Owner:           venue.ContextOwnerRecipe,
				ContextHash:     e.ContentHash,
				ConnectorConfig: governanceConfig(nil, profileID, e.Channel),
				PreviewKind:     e.GetPreview().GetKind(),
				PreviewURL:      e.GetPreview().GetUrl(),
			}
			if cerr := deps.ContentStore.CreateCollection(ctx, col); cerr != nil {
				return nil, fmt.Errorf("create collection %s: %w", e.Name, cerr)
			}
			result.Created = append(result.Created, e.Name)
			byName[e.Name] = col
			continue
		}

		// A row the recipe now declares becomes recipe-owned, whoever created
		// it. Declaring a collection in kapi.yaml is the act that claims it:
		// after this push git decides what that collection is, and the API
		// refusal keyed on Owner has something to refuse on behalf of.
		claiming := !venue.IsRecipeOwned(current.Owner)

		// Idempotence. The stored hash covers exactly the fields this entry
		// carries, so an unchanged hash means the recipe has not moved — and
		// rewriting the row would churn UpdatedAt for nothing. The governance
		// keys are still compared, because the profile id is minted server-side
		// and can change without the recipe changing (a hub rename, a first
		// bind after the project was claimed into a workspace).
		wantConfig := governanceConfig(current.ConnectorConfig, profileID, e.Channel)
		if !claiming && current.ContextHash == e.ContentHash &&
			maps.Equal(current.ConnectorConfig, wantConfig) {
			result.Unchanged = append(result.Unchanged, e.Name)
			continue
		}

		current.Context = maps.Clone(e.Coordinates)
		current.Owner = venue.ContextOwnerRecipe
		current.ContextHash = e.ContentHash
		current.ConnectorConfig = wantConfig
		// Written from the entry, including when the entry declares none: a
		// recipe that drops its `preview:` block means the collection no longer
		// has one, and leaving the old URL would keep offering a reading of
		// components that may no longer be published there.
		current.PreviewKind = e.GetPreview().GetKind()
		current.PreviewURL = e.GetPreview().GetUrl()
		if uerr := deps.ContentStore.UpdateCollection(ctx, current); uerr != nil {
			return nil, fmt.Errorf("update collection %s: %w", e.Name, uerr)
		}
		if claiming {
			result.Claimed = append(result.Claimed, e.Name)
		} else {
			result.Updated = append(result.Updated, e.Name)
		}
	}

	// Report, never delete. Only recipe-owned rows are reported: a
	// workspace-owned collection was never the recipe's to declare, so its
	// absence from a push says nothing about it.
	for name, col := range byName {
		if _, ok := declared[name]; ok {
			continue
		}
		if venue.IsRecipeOwned(col.Owner) {
			result.Undeclared = append(result.Undeclared, name)
		}
	}
	slices.Sort(result.Created)
	slices.Sort(result.Updated)
	slices.Sort(result.Claimed)
	slices.Sort(result.Unchanged)
	slices.Sort(result.Undeclared)

	if len(result.Undeclared) > 0 {
		slog.Info("sync push declares fewer collections than the server holds; keeping the rest",
			"project_id", projectID, "stream", stream,
			"undeclared", strings.Join(result.Undeclared, ","))
	}
	return result, nil
}

// governanceConfig returns the collection's connector config with the resolved
// governance written into the two keys core/profile reads: the bound profile id
// and the channel. Every other key is carried through untouched — a collection
// may also hold connector settings, and governance has no business dropping
// them.
//
// An empty profile id or channel REMOVES the key rather than storing an empty
// value, so "no voice bound at this point" reads the same as "never bound one".
func governanceConfig(current map[string]string, profileID, channel string) map[string]string {
	out := make(map[string]string, len(current)+2)
	maps.Copy(out, current)
	for key, value := range map[string]string{
		coreprofile.PropertyProfileID: profileID,
		coreprofile.PropertyChannel:   channel,
	} {
		if value == "" {
			delete(out, key)
			continue
		}
		out[key] = value
	}
	return out
}

// profileBinder resolves a context entry's voice profile to a brand-hub profile
// id, upserting the authored content the push carried. It caches by profile
// name, so several collections governed by one voice cost one upsert.
type profileBinder struct {
	deps        *WorkerDeps
	workspaceID string
	actorID     string
	byName      map[string]string
}

func newProfileBinder(deps *WorkerDeps, workspaceID, actorID string) *profileBinder {
	return &profileBinder{deps: deps, workspaceID: workspaceID, actorID: actorID, byName: map[string]string{}}
}

// bind returns the profile id governing this entry, creating or updating the
// workspace profile from the authored content the entry carried.
//
// This is what folding the brand push into the context content type buys: the
// voice arrives inside the push transaction rather than through a REST call
// beside it, so a push can no longer half-succeed with content stored against
// governance that never landed. Matching stays by NAME, exactly as the upsert
// endpoint does it, so the two paths cannot diverge on what "the same profile"
// means.
//
// An entry naming a profile the hub does not hold and carrying no content binds
// nothing. That is not an error: the recipe names a voice this workspace has
// never seen, and the collection is better ungoverned than governed by a
// same-named profile nobody meant.
func (b *profileBinder) bind(ctx context.Context, e *pb.SyncContextEntry) (string, error) {
	if e.VoiceProfile == "" || b.workspaceID == "" || b.deps.BrandStore == nil {
		return "", nil
	}
	if cached, ok := b.byName[e.VoiceProfile]; ok {
		return cached, nil
	}

	profiles, err := b.deps.BrandStore.ListProfiles(ctx, b.workspaceID)
	if err != nil {
		return "", fmt.Errorf("list brand profiles: %w", err)
	}
	var existing *coreprofile.VoiceProfile
	for _, p := range profiles {
		if p != nil && strings.EqualFold(p.Name, e.VoiceProfile) {
			existing = p
			break
		}
	}

	pushed, err := decodeVoiceProfile(e.VoiceProfileJson)
	if err != nil {
		return "", fmt.Errorf("decode voice profile %q: %w", e.VoiceProfile, err)
	}

	profileID, err := b.upsert(ctx, e.VoiceProfile, existing, pushed)
	if err != nil {
		return "", err
	}
	b.byName[e.VoiceProfile] = profileID
	return profileID, nil
}

// upsert applies the pushed profile content to the hub and returns the id to
// bind. With no pushed content it binds whatever the hub already holds under
// that name.
func (b *profileBinder) upsert(ctx context.Context, name string, existing, pushed *coreprofile.VoiceProfile) (string, error) {
	if pushed == nil {
		if existing == nil {
			return "", nil
		}
		return existing.ID, nil
	}

	now := time.Now().UTC()
	if existing == nil {
		created := *pushed
		created.ID = id.New()
		created.Name = name
		created.Scope = b.workspaceID
		created.Version = 1
		created.CreatedAt = now
		created.UpdatedAt = now
		created.CreatedBy = b.actorID
		if err := b.deps.BrandStore.CreateProfile(ctx, &created); err != nil {
			return "", fmt.Errorf("create brand profile %q: %w", name, err)
		}
		return created.ID, nil
	}

	// Unchanged authored content is a no-op, so a re-push of an unedited voice
	// does not manufacture a version. When it did change, UpdateProfile archives
	// the current state before bumping — a server-side edit is superseded by a
	// revertible version, never clobbered.
	if sameAuthoredVoice(existing, pushed) {
		return existing.ID, nil
	}
	updated := *existing
	updated.Description = pushed.Description
	updated.Tone = pushed.Tone
	updated.Style = pushed.Style
	updated.Vocabulary = pushed.Vocabulary
	updated.Examples = pushed.Examples
	updated.Locales = pushed.Locales
	updated.Channels = pushed.Channels
	updated.Personas = pushed.Personas
	updated.VersionNote = "superseded by kapi push"
	updated.UpdatedAt = now
	if err := b.deps.BrandStore.UpdateProfile(ctx, &updated); err != nil {
		return "", fmt.Errorf("update brand profile %q: %w", name, err)
	}
	return updated.ID, nil
}

// decodeVoiceProfile reads the authored voice content a context entry carried.
func decodeVoiceProfile(raw []byte) (*coreprofile.VoiceProfile, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var p coreprofile.VoiceProfile
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// sameAuthoredVoice compares the authored surface of two profiles — everything
// a recipe can write — ignoring the server-managed metadata (id, workspace,
// version, timestamps) that would otherwise make every comparison unequal.
func sameAuthoredVoice(a, b *coreprofile.VoiceProfile) bool {
	authored := func(p *coreprofile.VoiceProfile) string {
		snapshot := *p
		snapshot.ID = ""
		snapshot.Scope = ""
		snapshot.Version = 0
		snapshot.VersionNote = ""
		snapshot.CreatedAt = time.Time{}
		snapshot.UpdatedAt = time.Time{}
		snapshot.CreatedBy = ""
		encoded, err := json.Marshal(snapshot)
		if err != nil {
			return ""
		}
		return string(encoded)
	}
	left, right := authored(a), authored(b)
	return left != "" && left == right
}
