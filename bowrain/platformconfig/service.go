package platformconfig

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
)

// errNoStore is returned when a write is attempted on an env-only service (no
// database configured).
var errNoStore = errors.New("platformconfig: no persistent store configured")

// Service is the runtime read/write facade over the platform_config store. It
// caches the (small) settings table in memory and serves typed accessors with
// env/product Defaults as the fallback, so a read is never a DB hit on the
// request path. The cache is refreshed at startup and whenever a
// platform_config.changed event fires (so a change made in ctrl on one instance
// propagates to every server and worker without a redeploy).
//
// Accessors are ctx-free and safe before the first Refresh: an unloaded cache
// simply yields Defaults, which is the correct bootstrap behavior.
type Service struct {
	store    *Store
	defaults Defaults

	mu    sync.RWMutex
	cache map[string]json.RawMessage
}

// NewService constructs the service. Call Refresh(ctx) once at startup to warm
// the cache; until then, accessors return Defaults.
func NewService(store *Store, defaults Defaults) *Service {
	return &Service{store: store, defaults: defaults, cache: map[string]json.RawMessage{}}
}

// Defaults returns the bootstrap defaults (used to render "(default)" hints).
func (s *Service) DefaultsValue() Defaults { return s.defaults }

// Persistent reports whether the service is backed by a DB store. When false
// (no database), it serves Defaults only and writes are rejected.
func (s *Service) Persistent() bool { return s.store != nil }

// Refresh reloads the whole settings table into the in-memory cache. Called at
// startup and on a platform_config.changed event. A no-op (env-only) service
// with no store refreshes to nothing and keeps serving Defaults.
func (s *Service) Refresh(ctx context.Context) error {
	if s.store == nil {
		return nil
	}
	all, err := s.store.GetAll(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cache = all
	s.mu.Unlock()
	return nil
}

// Set writes one or more keys, then refreshes this instance's cache. The caller
// is responsible for publishing platform_config.changed so other instances also
// refresh.
func (s *Service) Set(ctx context.Context, updates map[string]json.RawMessage, updatedBy string) error {
	if s.store == nil {
		return errNoStore
	}
	if err := s.store.SetMany(ctx, updates, updatedBy); err != nil {
		return err
	}
	return s.Refresh(ctx)
}

// Delete resets a key to its bootstrap default.
func (s *Service) Delete(ctx context.Context, key string) error {
	if s.store == nil {
		return errNoStore
	}
	if err := s.store.Delete(ctx, key); err != nil {
		return err
	}
	return s.Refresh(ctx)
}

func (s *Service) raw(key string) (json.RawMessage, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.cache[key]
	return v, ok
}

func (s *Service) getString(key, def string) string {
	v, ok := s.raw(key)
	if !ok {
		return def
	}
	var out string
	if err := json.Unmarshal(v, &out); err != nil || out == "" {
		return def
	}
	return out
}

func (s *Service) getBool(key string, def bool) bool {
	v, ok := s.raw(key)
	if !ok {
		return def
	}
	var out bool
	if err := json.Unmarshal(v, &out); err != nil {
		return def
	}
	return out
}

func (s *Service) getInt(key string, def int) int {
	v, ok := s.raw(key)
	if !ok {
		return def
	}
	var out int
	if err := json.Unmarshal(v, &out); err != nil || out <= 0 {
		return def
	}
	return out
}

func (s *Service) getStringSlice(key string) []string {
	v, ok := s.raw(key)
	if !ok {
		return nil
	}
	var out []string
	if err := json.Unmarshal(v, &out); err != nil {
		return nil
	}
	return out
}

func (s *Service) getStringBoolMap(key string) map[string]bool {
	v, ok := s.raw(key)
	if !ok {
		return map[string]bool{}
	}
	var out map[string]bool
	if err := json.Unmarshal(v, &out); err != nil || out == nil {
		return map[string]bool{}
	}
	return out
}

// --- typed accessors ----------------------------------------------------------

func (s *Service) AIProvider() string { return s.getString(KeyAIProvider, s.defaults.AIProvider) }
func (s *Service) AIDefaultModel() string {
	return s.getString(KeyAIDefaultModel, s.defaults.AIDefaultModel)
}
func (s *Service) AIBaseURL() string      { return s.getString(KeyAIBaseURL, s.defaults.AIBaseURL) }
func (s *Service) AICustomerChoice() bool { return s.getBool(KeyAICustomerChoice, false) }

// AIEnabledModels returns the admin-onboarded model ids. When none are set it
// falls back to just the default model, so the platform always has at least one
// enabled model.
func (s *Service) AIEnabledModels() []string {
	models := s.getStringSlice(KeyAIEnabledModels)
	if len(models) == 0 {
		if def := s.AIDefaultModel(); def != "" {
			return []string{def}
		}
		return nil
	}
	return models
}

// IsModelEnabled reports whether a model id is in the enabled set — the guard for
// the per-workspace customer model choice.
func (s *Service) IsModelEnabled(id string) bool {
	return slices.Contains(s.AIEnabledModels(), id)
}

// ResolveWorkspaceModel returns the model a workspace should use for platform AI,
// applying its stored preference only when the admin has enabled customer model
// choice and the preference is a currently-enabled model; otherwise it returns
// the platform default. This is the single source of truth for the per-workspace
// model policy, shared by the synchronous editor path (server) and the async
// translation worker so interactive and batch translation resolve identically.
func (s *Service) ResolveWorkspaceModel(preferred string) string {
	if preferred != "" && s.AICustomerChoice() && s.IsModelEnabled(preferred) {
		return preferred
	}
	return s.AIDefaultModel()
}

func (s *Service) SignupsOpen() bool { return s.getBool(KeySignupsOpen, s.defaults.SignupsOpen) }

// ModelSweepsEnabled reports whether measured steerability (model
// recommendation sweeps) is on for this instance. Default OFF — no env
// bootstrap; only an explicit ctrl write enables it. Both the sweep endpoints
// and the worker's consumption of a measured recommendation gate on it.
func (s *Service) ModelSweepsEnabled() bool { return s.getBool(KeyModelSweeps, false) }

func (s *Service) Maintenance() Maintenance {
	return Maintenance{
		Enabled: s.getBool(KeyMaintenanceOn, false),
		Message: s.getString(KeyMaintenanceMsg, ""),
	}
}

func (s *Service) DefaultPlan() string { return s.getString(KeyDefaultPlan, s.defaults.DefaultPlan) }
func (s *Service) TrialDays() int      { return s.getInt(KeyTrialDays, s.defaults.TrialDays) }

// GlobalFeatures returns the instance-wide feature default toggles. These layer
// UNDER per-workspace overrides (override wins) and OVER the plan matrix.
func (s *Service) GlobalFeatures() map[string]bool { return s.getStringBoolMap(KeyFeatures) }

// AISettings returns the resolved AI configuration.
func (s *Service) AISettings() AISettings {
	return AISettings{
		Provider:       s.AIProvider(),
		DefaultModel:   s.AIDefaultModel(),
		BaseURL:        s.AIBaseURL(),
		EnabledModels:  s.AIEnabledModels(),
		CustomerChoice: s.AICustomerChoice(),
	}
}

// enabledModelDetails resolves enabled ids to catalog entries (unknown ids become
// a bare display entry), for the public/customer model picker.
func (s *Service) enabledModelDetails() []Model {
	provider := s.AIProvider()
	var out []Model
	for _, id := range s.AIEnabledModels() {
		matched := false
		for _, m := range Catalog(provider) {
			if m.ID == id {
				out = append(out, m)
				matched = true
				break
			}
		}
		if !matched {
			out = append(out, Model{ID: id, Name: id})
		}
	}
	return out
}

// Snapshot returns the full admin-facing configuration, including the model
// catalog for the current provider.
func (s *Service) Snapshot() Snapshot {
	return Snapshot{
		AI:                s.AISettings(),
		Signups:           SignupSettings{Open: s.SignupsOpen()},
		Maintenance:       s.Maintenance(),
		WorkspaceDefaults: WorkspaceDefaults{Plan: s.DefaultPlan(), TrialDays: s.TrialDays()},
		Features:          s.GlobalFeatures(),
		ModelSweeps:       ModelSweepSettings{Enabled: s.ModelSweepsEnabled()},
		ModelCatalog:      Catalog(s.AIProvider()),
	}
}

// PublicSnapshot returns the unauthenticated, public-safe subset.
func (s *Service) PublicSnapshot() PublicSnapshot {
	ps := PublicSnapshot{
		SignupsOpen:    s.SignupsOpen(),
		Maintenance:    s.Maintenance(),
		CustomerChoice: s.AICustomerChoice(),
		DefaultModel:   s.AIDefaultModel(),
	}
	// Only expose the enabled model list publicly when customers may choose.
	if ps.CustomerChoice {
		ps.EnabledModels = s.enabledModelDetails()
	}
	return ps
}
