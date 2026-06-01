package brand

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
)

// Well-known property keys for brand voice bindings on Project.Properties,
// Stream.Properties, and Collection.ConnectorConfig maps.
const (
	PropertyProfileID = "brand_voice_profile_id"
	PropertyChannel   = "brand_voice_channel"
)

// BrandVoiceBinding associates a brand voice profile with an organizational scope.
type BrandVoiceBinding struct {
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

	// Locale is the target locale for locale-specific override resolution.
	Locale model.LocaleID
}

// ProfileResolver resolves the effective brand voice profile for a given context.
type ProfileResolver interface {
	ResolveProfile(ctx context.Context, rc ResolveContext) (*VoiceProfile, error)
}

// StoreProfileResolver implements ProfileResolver using a BrandStore.
type StoreProfileResolver struct {
	Store BrandStore
}

// ResolveProfile resolves the most specific brand voice profile from the context
// hierarchy and applies locale + channel overrides.
func (r *StoreProfileResolver) ResolveProfile(ctx context.Context, rc ResolveContext) (*VoiceProfile, error) {
	return ResolveProfileFromContext(ctx, rc, r.Store)
}
