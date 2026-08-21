// Package brandscope bridges the framework's hierarchical brand-voice resolver
// (core/brand) to the bowrain platform's organizational scopes. It reads the
// brand-voice binding that a project, stream, or collection carries in its
// Properties/ConnectorConfig map, plus the workspace-level default, and feeds
// them to coreprofile.ResolveProfileFromContext so a surface resolves the most
// specific profile without the caller knowing a profile ID.
//
// The resolution ladder (most specific wins) is:
//
//	explicit → collection → stream → project → workspace
//
// See core/brand/resolve.go for the ladder itself; this package only populates
// the coreprofile.ResolveContext from the store types.
package brandscope

import (
	"context"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// Scope identifies the organizational-hierarchy positions a brand check runs
// within. Empty fields are skipped during resolution, so a caller populates
// only the scopes it actually knows.
type Scope struct {
	// ExplicitProfileID, when set, is the top of the ladder: it short-circuits
	// every store read and always wins.
	ExplicitProfileID string

	// WorkspaceID is the workspace whose default profile forms the ladder's
	// base. When empty, the resolved project's own WorkspaceID is used.
	WorkspaceID string

	ProjectID    string
	Stream       string
	CollectionID string

	// Locale and Channel drive the framework resolver's locale/channel overrides
	// on the selected profile. Channel is applied last, so an explicit channel
	// beats one bound via properties.
	Locale  model.LocaleID
	Channel string

	// Persona selects an author persona override on the resolved profile,
	// applied after channel and bounded by the brand's guardrails. It is
	// normally supplied explicitly at check time (e.g. an MCP scoring call)
	// rather than bound to a scope.
	Persona string
}

// ScopeStore is the subset of store.ContentStore brandscope needs to read the
// project, stream, and collection carrying a brand-voice binding.
// store.ContentStore satisfies it.
type ScopeStore interface {
	GetProject(ctx context.Context, id string) (*store.Project, error)
	GetStream(ctx context.Context, projectID, name string) (*store.Stream, error)
	GetCollection(ctx context.Context, projectID, collectionID string) (*store.Collection, error)
}

// WorkspaceDefault resolves the workspace-level default brand-voice profile ID.
// It is implemented over the auth store where a workspace lookup is available
// and may be nil where it is not (in which case the workspace rung is skipped).
type WorkspaceDefault interface {
	WorkspaceBrandProfileID(ctx context.Context, workspaceID string) (string, error)
}

// Resolve populates a coreprofile.ResolveContext from the given scopes and resolves
// the effective profile through the hierarchical ladder. An explicit profile ID
// short-circuits the store reads and always wins; otherwise the project,
// stream, and collection bindings and the workspace default are consulted in
// specificity order. Returns (nil, nil) when nothing is bound at any level —
// the same "no profile" outcome callers already handle.
//
// Store-read failures for a scope are non-fatal: that rung is simply skipped,
// so a missing stream or an unreadable collection degrades to the next-broader
// binding rather than failing the check.
func Resolve(ctx context.Context, cs ScopeStore, wd WorkspaceDefault, bs coreprofile.Store, sc Scope) (*coreprofile.VoiceProfile, error) {
	rc := coreprofile.ResolveContext{
		ExplicitProfileID: sc.ExplicitProfileID,
		Locale:            sc.Locale,
	}
	if sc.ExplicitProfileID == "" {
		populateContext(ctx, cs, wd, &rc, sc)
	}

	profile, err := coreprofile.ResolveProfileFromContext(ctx, rc, bs)
	if err != nil || profile == nil {
		return profile, err
	}
	// An explicit per-call channel/persona wins over anything bound via
	// properties. Persona is applied after channel, inside the brand guardrails.
	if sc.Channel != "" || sc.Persona != "" {
		profile = coreprofile.ResolveProfile(profile, "", sc.Channel, sc.Persona)
	}
	return profile, nil
}

// populateContext fills rc's Project/Stream/Collection maps and workspace
// default from the stores. The project is read first so its WorkspaceID can
// stand in for an unspecified Scope.WorkspaceID (the common case in surfaces
// that only know the project).
func populateContext(ctx context.Context, cs ScopeStore, wd WorkspaceDefault, rc *coreprofile.ResolveContext, sc Scope) {
	workspaceID := sc.WorkspaceID

	if cs != nil && sc.ProjectID != "" {
		if p, err := cs.GetProject(ctx, sc.ProjectID); err == nil && p != nil {
			rc.ProjectProperties = p.Properties
			if workspaceID == "" {
				workspaceID = p.WorkspaceID
			}
		}
		if sc.Stream != "" {
			if st, err := cs.GetStream(ctx, sc.ProjectID, sc.Stream); err == nil && st != nil {
				rc.StreamProperties = st.Properties
			}
		}
		if sc.CollectionID != "" {
			if col, err := cs.GetCollection(ctx, sc.ProjectID, sc.CollectionID); err == nil && col != nil {
				rc.CollectionConfig = col.ConnectorConfig
			}
		}
	}

	if wd != nil && workspaceID != "" {
		if id, err := wd.WorkspaceBrandProfileID(ctx, workspaceID); err == nil {
			rc.RootProfileID = id
		}
	}
}
