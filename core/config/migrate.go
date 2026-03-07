package config

import (
	"fmt"
	"sync"
)

// MigrationFunc transforms a spec from one version to the next.
type MigrationFunc func(spec map[string]any) (map[string]any, error)

// Migration represents a single version upgrade step.
type Migration struct {
	FromVersion string
	ToVersion   string
	Migrate     MigrationFunc
}

// MigrationRegistry holds version upgrade chains keyed by resource key.
type MigrationRegistry struct {
	mu     sync.RWMutex
	chains map[string][]Migration // keyed by "{namespace}/{resource}"
}

// NewMigrationRegistry creates an empty MigrationRegistry.
func NewMigrationRegistry() *MigrationRegistry {
	return &MigrationRegistry{
		chains: make(map[string][]Migration),
	}
}

// Register adds a migration step for a resource.
func (r *MigrationRegistry) Register(m Migration) error {
	from, err := ParseAPIVersion(m.FromVersion)
	if err != nil {
		return fmt.Errorf("migration from: %w", err)
	}
	to, err := ParseAPIVersion(m.ToVersion)
	if err != nil {
		return fmt.Errorf("migration to: %w", err)
	}
	if from.ResourceKey() != to.ResourceKey() {
		return fmt.Errorf("migration resource mismatch: %s vs %s", from.ResourceKey(), to.ResourceKey())
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	key := from.ResourceKey()
	r.chains[key] = append(r.chains[key], m)
	return nil
}

// Upgrade migrates an envelope's spec to the latest version by walking the
// migration chain. The envelope's APIVersion and Spec are updated in place.
// Returns nil if no migration is needed. Returns an error if the version
// is newer than known or if a migration step fails.
func (r *MigrationRegistry) Upgrade(env *Envelope) error {
	av, err := ParseAPIVersion(env.APIVersion)
	if err != nil {
		return err
	}

	r.mu.RLock()
	chain := r.chains[av.ResourceKey()]
	r.mu.RUnlock()

	if len(chain) == 0 {
		return nil // no migrations registered
	}

	// Find the highest known version from registered migrations
	var maxVersion int
	for _, m := range chain {
		to, _ := ParseAPIVersion(m.ToVersion)
		if to.Version > maxVersion {
			maxVersion = to.Version
		}
		from, _ := ParseAPIVersion(m.FromVersion)
		if from.Version > maxVersion {
			maxVersion = from.Version
		}
	}

	if av.Version > maxVersion {
		return fmt.Errorf("apiVersion %s is newer than supported (max: %s/%s-v%d)",
			env.APIVersion, av.Namespace, av.Resource, maxVersion)
	}

	// Walk the chain from current version forward
	current := env.APIVersion
	for {
		var found *Migration
		for i := range chain {
			if chain[i].FromVersion == current {
				found = &chain[i]
				break
			}
		}
		if found == nil {
			break // no more migrations
		}

		newSpec, err := found.Migrate(env.Spec)
		if err != nil {
			return fmt.Errorf("migrate %s -> %s: %w", found.FromVersion, found.ToVersion, err)
		}
		env.Spec = newSpec
		env.APIVersion = found.ToVersion
		current = found.ToVersion
	}

	return nil
}

// LatestVersion returns the highest registered version for a resource key,
// or 0 if no migrations exist.
func (r *MigrationRegistry) LatestVersion(resourceKey string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	chain := r.chains[resourceKey]
	max := 0
	for _, m := range chain {
		to, err := ParseAPIVersion(m.ToVersion)
		if err == nil && to.Version > max {
			max = to.Version
		}
	}
	return max
}

// DefaultMigrations is the global migration registry.
var DefaultMigrations = NewMigrationRegistry()
