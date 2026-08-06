package profile

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
)

// Well-known property keys for voice profile bindings on Project.Properties,
// Stream.Properties, and Collection.ConnectorConfig maps. The key strings are
// persisted in platform stores and stay as written.
const (
	PropertyProfileID = "brand_voice_profile_id"
	PropertyChannel   = "brand_voice_channel"
	PropertyPersona   = "brand_voice_persona"
)

// ScopeBinding associates a voice profile with an organizational scope.
type ScopeBinding struct {
	ProfileID string `json:"profile_id" yaml:"profile_id"`
	Channel   string `json:"channel,omitempty" yaml:"channel,omitempty"` // maps to ChannelOverride key
}

// ResolveContext holds the organizational context for hierarchical profile resolution.
// Fields are populated from the workspace, project, stream, and collection in scope.
type ResolveContext struct {
	// ExplicitProfileID takes priority over all other resolution levels.
	ExplicitProfileID string

	// WorkspaceProfileID is the workspace-level default profile.
	WorkspaceProfileID string

	// ProjectProperties is the Project.Properties map.
	ProjectProperties map[string]string

	// StreamProperties is the Stream.Properties map.
	StreamProperties map[string]string

	// CollectionConfig is the Collection.ConnectorConfig map.
	CollectionConfig map[string]string

	// CollectionProfile is a profile already loaded for the collection in
	// scope, bound at the same tier as CollectionConfig[PropertyProfileID] and
	// consulted before it. It exists because a binding is not always a store
	// row: a kapi recipe binds the voice governing a collection as a profile
	// file or a starter pack (core/project, `profiles:`), and the caller has
	// therefore already resolved it. Supplying it here rather than resolving
	// beside this chain is what keeps one precedence model — an explicit
	// profile still wins over it, and stream, project and workspace bindings
	// still sit under it.
	CollectionProfile *VoiceProfile

	// Locale is the target locale for locale-specific override resolution.
	Locale model.LocaleID

	// Channel, when set, selects a channel override and takes priority over any
	// channel bound via the scope maps — the `--channel` flag's tier, mirroring
	// Persona. A channel bound to a scope describes where that content is
	// published; this one is the caller overriding it for a single call.
	Channel string

	// Persona, when set, selects an author persona override and takes priority
	// over any persona bound via the scope maps. A persona is normally supplied
	// explicitly at check time rather than bound to a scope.
	Persona string
}

// ProfileResolver resolves the effective voice profile for a given context.
type ProfileResolver interface {
	ResolveProfile(ctx context.Context, rc ResolveContext) (*VoiceProfile, error)
}

// StoreProfileResolver implements ProfileResolver using a Store.
type StoreProfileResolver struct {
	Store Store
}

// ResolveProfile resolves the most specific voice profile from the context
// hierarchy and applies locale + channel overrides.
func (r *StoreProfileResolver) ResolveProfile(ctx context.Context, rc ResolveContext) (*VoiceProfile, error) {
	return ResolveProfileFromContext(ctx, rc, r.Store)
}
