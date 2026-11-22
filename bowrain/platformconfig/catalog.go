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
	TierFlagship ModelTier = "flagship" // highest quality/cost (Opus, GPT-6 Astra)
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
	// Claude models and the OpenAI models). Onboarding it without that
	// subscription yields a 403 at invoke time, and the UI warns accordingly.
	RequiresMarketplace bool `json:"requires_marketplace,omitempty"`
}

// BedrockCatalog is the maintained set of text models the platform can onboard
// from AWS Bedrock in eu-north-1: the Anthropic Claude models in the eu.
// cross-region inference zone, and the OpenAI GPT models, which Bedrock serves
// only through global. inference profiles (global routing, not EU-only). The
// list is checked against the production account's inference profiles.
// On-demand-invokable models carry no marketplace flag; the rest need an
// account-level AWS Marketplace agreement before they invoke, and are flagged so
// the admin knows to accept it in AWS before onboarding.
//
// Maintenance note: when AWS adds a model to Bedrock, add a row here (and drop
// the marketplace flag once it invokes on-demand); when Bedrock stops serving
// one, remove its row. Every row must resolve to an entry in the framework's
// model catalog (providers/ai/models.json), which a test enforces. This list
// changes on a model refresh, alongside that catalog.
var BedrockCatalog = []Model{
	{ID: "eu.anthropic.claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Tier: TierBalanced},
	{ID: "eu.anthropic.claude-sonnet-4-5-20250929-v1:0", Name: "Claude Sonnet 4.5", Tier: TierBalanced},
	{ID: "eu.anthropic.claude-opus-4-6-v1", Name: "Claude Opus 4.6", Tier: TierFlagship},
	{ID: "eu.anthropic.claude-opus-4-5-20251101-v1:0", Name: "Claude Opus 4.5", Tier: TierFlagship},
	{ID: "eu.anthropic.claude-haiku-4-5-20251001-v1:0", Name: "Claude Haiku 4.5", Tier: TierFast},
	{ID: "eu.anthropic.claude-sonnet-5-5", Name: "Claude Sonnet 5.5", Tier: TierBalanced, RequiresMarketplace: true},
	{ID: "eu.anthropic.claude-sonnet-5", Name: "Claude Sonnet 5", Tier: TierFlagship, RequiresMarketplace: true},
	{ID: "eu.anthropic.claude-haiku-5-5", Name: "Claude Haiku 5.5", Tier: TierFast, RequiresMarketplace: true},
	{ID: "eu.anthropic.claude-opus-5-5", Name: "Claude Opus 5.5", Tier: TierFlagship, RequiresMarketplace: true},
	{ID: "eu.anthropic.claude-opus-5", Name: "Claude Opus 5", Tier: TierFlagship, RequiresMarketplace: true},
	{ID: "eu.anthropic.claude-opus-4-8", Name: "Claude Opus 4.8", Tier: TierFlagship, RequiresMarketplace: true},
	{ID: "eu.anthropic.claude-opus-4-7", Name: "Claude Opus 4.7", Tier: TierFlagship, RequiresMarketplace: true},
	{ID: "global.openai.gpt-6.1-terra", Name: "GPT-6.1 Terra", Tier: TierBalanced, RequiresMarketplace: true},
	{ID: "global.openai.gpt-6.1-sol", Name: "GPT-6.1 Sol", Tier: TierFlagship, RequiresMarketplace: true},
	{ID: "global.openai.gpt-6-astra", Name: "GPT-6 Astra", Tier: TierFlagship, RequiresMarketplace: true},
	{ID: "global.openai.gpt-6-luna", Name: "GPT-6 Luna", Tier: TierFast, RequiresMarketplace: true},
	{ID: "global.openai.gpt-5.6-terra", Name: "GPT-5.6 Terra", Tier: TierBalanced, RequiresMarketplace: true},
}

// Catalog returns the model catalog for a provider. Only "bedrock" has a curated
// catalog; other providers return an empty list (the admin sets a free-text
// model), which serializes as [] rather than null.
func Catalog(provider string) []Model {
	if provider == "bedrock" {
		return BedrockCatalog
	}
	return []Model{}
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
