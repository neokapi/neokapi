// Package platformconfig is the instance-wide (singleton) settings store for the
// Bowrain platform: the admin-controlled configuration surfaced in the ctrl
// control plane and applied at runtime by the server and worker.
//
// It is deliberately a flat key/value store (VSCode-settings style) rather than
// a table-per-concern: instance knobs never stay singular, so a generic KV store
// means each new setting is a key + a typed accessor, not a migration. Values are
// JSON; unset keys fall back to process-env/product Defaults, so an
// un-provisioned instance behaves exactly as it did before this package existed
// (DB overrides env, never the reverse).
package platformconfig

// ModelTier is a coarse quality/cost band, for grouping models in the admin UI.
type ModelTier string

const (
	TierFast     ModelTier = "fast"     // cheapest/fastest (Haiku)
	TierBalanced ModelTier = "balanced" // the translation default band (Sonnet)
	TierFlagship ModelTier = "flagship" // highest quality/cost (Opus, Sonnet 5)
)

// Model describes one entry in the platform's model catalog. The catalog is a
// static, code-maintained list (NOT a live Bedrock API call): the admin onboards
// a subset of it as the enabled set. Keeping it static means no runtime Bedrock
// dependency, no extra IAM, and no failure mode when the catalog renders.
type Model struct {
	// ID is the Bedrock cross-region inference-profile id used as the model id
	// (e.g. "eu.anthropic.claude-sonnet-4-6"). It is what gets stored as the
	// platform model and passed to the provider.
	ID string `json:"id"`
	// Name is the human display name.
	Name string `json:"name"`
	// Tier is the quality/cost band.
	Tier ModelTier `json:"tier"`
	// RequiresMarketplace flags a model that is catalog-listed but needs an extra
	// account-level AWS Marketplace subscription before it will invoke (the newest
	// Claude models). Onboarding it without that subscription yields a 403 at
	// invoke time — the UI warns accordingly.
	RequiresMarketplace bool `json:"requires_marketplace,omitempty"`
}

// BedrockAnthropicCatalog is the maintained set of Anthropic Claude models
// available on AWS Bedrock in the eu-north-1 cross-region (eu.) inference zone,
// empirically confirmed against the production account. On-demand-invokable
// models carry no marketplace flag; the newest ones (Sonnet 5, Opus 4.7/4.8)
// require a Marketplace subscription and are flagged so the admin knows to enable
// them in AWS before onboarding.
//
// Maintenance note: when AWS adds a model to Bedrock, add a row here (and drop
// the marketplace flag once it invokes on-demand). This is intentionally the one
// place that changes on a model refresh.
var BedrockAnthropicCatalog = []Model{
	{ID: "eu.anthropic.claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Tier: TierBalanced},
	{ID: "eu.anthropic.claude-sonnet-4-5-20250929-v1:0", Name: "Claude Sonnet 4.5", Tier: TierBalanced},
	{ID: "eu.anthropic.claude-opus-4-6-v1", Name: "Claude Opus 4.6", Tier: TierFlagship},
	{ID: "eu.anthropic.claude-opus-4-5-20251101-v1:0", Name: "Claude Opus 4.5", Tier: TierFlagship},
	{ID: "eu.anthropic.claude-haiku-4-5-20251001-v1:0", Name: "Claude Haiku 4.5", Tier: TierFast},
	{ID: "eu.anthropic.claude-sonnet-5", Name: "Claude Sonnet 5", Tier: TierFlagship, RequiresMarketplace: true},
	{ID: "eu.anthropic.claude-opus-4-8", Name: "Claude Opus 4.8", Tier: TierFlagship, RequiresMarketplace: true},
	{ID: "eu.anthropic.claude-opus-4-7", Name: "Claude Opus 4.7", Tier: TierFlagship, RequiresMarketplace: true},
}

// Catalog returns the model catalog for a provider. Only "bedrock" has a curated
// catalog today; other providers return nil (the admin sets a free-text model).
func Catalog(provider string) []Model {
	if provider == "bedrock" {
		return BedrockAnthropicCatalog
	}
	return nil
}

// InCatalog reports whether a model id exists in the given provider's catalog.
func InCatalog(provider, id string) bool {
	for _, m := range Catalog(provider) {
		if m.ID == id {
			return true
		}
	}
	return false
}
