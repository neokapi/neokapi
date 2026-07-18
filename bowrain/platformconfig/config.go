package platformconfig

// Config keys. Flat, dotted, VSCode-settings style. Each maps to a JSON value in
// the platform_config table; the Service exposes a typed accessor per key and
// falls back to Defaults when a key is unset.
const (
	KeyAIProvider       = "ai.provider"            // string: platform AI provider id (e.g. "bedrock")
	KeyAIDefaultModel   = "ai.default_model"       // string: default model id
	KeyAIBaseURL        = "ai.base_url"            // string: optional provider endpoint override
	KeyAIEnabledModels  = "ai.enabled_models"      // []string: admin-onboarded model ids
	KeyAICustomerChoice = "ai.customer_choice"     // bool: let workspaces pick from the enabled set
	KeySignupsOpen      = "signups.open"           // bool: allow new signups / workspace creation
	KeyMaintenanceOn    = "maintenance.enabled"    // bool: maintenance banner active
	KeyMaintenanceMsg   = "maintenance.message"    // string: maintenance banner text
	KeyDefaultPlan      = "workspace.default_plan" // string: plan for new workspaces
	KeyTrialDays        = "workspace.trial_days"   // int: trial length for new workspaces
	KeyFeatures         = "features"               // map[string]bool: instance-wide feature default toggles
	// KeyModelSweeps gates measured steerability (model recommendation sweeps):
	// both the sweep endpoints (refresh) and the worker's consumption of a
	// measured recommendation respect it. Default OFF — the founder flips it in
	// ctrl when ready. Platform QC: sweeps run on the platform provider and
	// never deduct customer credits.
	KeyModelSweeps = "model_sweeps.enabled" // bool: enable model recommendation sweeps
)

// Defaults are the bootstrap values used when a key is not set in the DB. They
// are sourced from process env (BOWRAIN_PLATFORM_*) and the historical product
// defaults, so an un-provisioned instance behaves exactly as before this store
// existed. DB values always override these.
type Defaults struct {
	AIProvider     string `json:"ai_provider"`
	AIDefaultModel string `json:"ai_default_model"`
	AIBaseURL      string `json:"ai_base_url"`
	SignupsOpen    bool   `json:"signups_open"`
	DefaultPlan    string `json:"default_plan"`
	TrialDays      int    `json:"trial_days"`
}

// AISettings is the resolved AI configuration surfaced to the admin UI.
type AISettings struct {
	Provider       string   `json:"provider"`
	DefaultModel   string   `json:"default_model"`
	BaseURL        string   `json:"base_url,omitempty"`
	EnabledModels  []string `json:"enabled_models"`
	CustomerChoice bool     `json:"customer_choice"`
}

// Maintenance is the resolved maintenance-mode state.
type Maintenance struct {
	Enabled bool   `json:"enabled"`
	Message string `json:"message,omitempty"`
}

// WorkspaceDefaults is the resolved new-workspace provisioning configuration.
type WorkspaceDefaults struct {
	Plan      string `json:"plan"`
	TrialDays int    `json:"trial_days"`
}

// Snapshot is the full admin-facing view of platform configuration: the resolved
// settings plus the model catalog for the current provider (so the UI can render
// onboard/offboard without a separate call).
type Snapshot struct {
	AI                AISettings         `json:"ai"`
	Signups           SignupSettings     `json:"signups"`
	Maintenance       Maintenance        `json:"maintenance"`
	WorkspaceDefaults WorkspaceDefaults  `json:"workspace_defaults"`
	Features          map[string]bool    `json:"features"`
	ModelSweeps       ModelSweepSettings `json:"model_sweeps"`
	ModelCatalog      []Model            `json:"model_catalog"`
}

// SignupSettings is the resolved signup gate.
type SignupSettings struct {
	Open bool `json:"open"`
}

// ModelSweepSettings is the resolved measured-steerability gate.
type ModelSweepSettings struct {
	Enabled bool `json:"enabled"`
}

// PublicSnapshot is the unauthenticated, public-safe subset (served from the web
// app's bootstrap /info call): whether signups are open, any maintenance banner,
// and — for the customer model-choice feature — the enabled models and default.
type PublicSnapshot struct {
	SignupsOpen    bool        `json:"signups_open"`
	Maintenance    Maintenance `json:"maintenance"`
	CustomerChoice bool        `json:"ai_customer_choice"`
	EnabledModels  []Model     `json:"ai_enabled_models,omitempty"`
	DefaultModel   string      `json:"ai_default_model,omitempty"`
}
