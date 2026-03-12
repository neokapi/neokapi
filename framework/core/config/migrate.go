package config

import (
	"fmt"
	"sync"
)

// MigrationFunc transforms a spec from one version to the next.
type MigrationFunc func(spec map[string]any) (map[string]any, error)

// Migration represents a single version upgrade step for a specific kind.
type Migration struct {
	Kind        Kind
	FromVersion int
	ToVersion   int
	Migrate     MigrationFunc
}

// MigrationRegistry holds version upgrade chains keyed by Kind.
type MigrationRegistry struct {
	mu     sync.RWMutex
	chains map[Kind][]Migration
}

// NewMigrationRegistry creates an empty MigrationRegistry.
func NewMigrationRegistry() *MigrationRegistry {
	return &MigrationRegistry{
		chains: make(map[Kind][]Migration),
	}
}

// Register adds a migration step for a kind.
func (r *MigrationRegistry) Register(m Migration) error {
	if m.Kind == "" {
		return fmt.Errorf("migration kind is required")
	}
	if m.FromVersion < 1 {
		return fmt.Errorf("migration fromVersion must be >= 1")
	}
	if m.ToVersion <= m.FromVersion {
		return fmt.Errorf("migration toVersion must be > fromVersion")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.chains[m.Kind] = append(r.chains[m.Kind], m)
	return nil
}

// Upgrade migrates an envelope's spec to the latest version by walking the
// migration chain. The envelope's APIVersion and Spec are updated in place.
// Returns nil if no migration is needed. Returns an error if the version
// is newer than known or if a migration step fails.
func (r *MigrationRegistry) Upgrade(env *Envelope) error {
	version, err := ParseAPIVersion(env.APIVersion)
	if err != nil {
		return err
	}

	r.mu.RLock()
	chain := r.chains[env.Kind]
	r.mu.RUnlock()

	if len(chain) == 0 {
		return nil // no migrations registered
	}

	// Find the highest known version from registered migrations
	var maxVersion int
	for _, m := range chain {
		if m.ToVersion > maxVersion {
			maxVersion = m.ToVersion
		}
		if m.FromVersion > maxVersion {
			maxVersion = m.FromVersion
		}
	}

	if version > maxVersion {
		return fmt.Errorf("kind %s apiVersion v%d is newer than supported (max: v%d)",
			env.Kind, version, maxVersion)
	}

	// Walk the chain from current version forward
	current := version
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
			return fmt.Errorf("migrate %s v%d -> v%d: %w", env.Kind, found.FromVersion, found.ToVersion, err)
		}
		env.Spec = newSpec
		env.APIVersion = FormatAPIVersion(found.ToVersion)
		current = found.ToVersion
	}

	return nil
}

// LatestVersion returns the highest registered version for a kind,
// or 0 if no migrations exist.
func (r *MigrationRegistry) LatestVersion(kind Kind) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	chain := r.chains[kind]
	max := 0
	for _, m := range chain {
		if m.ToVersion > max {
			max = m.ToVersion
		}
	}
	return max
}

// DefaultMigrations is the global migration registry.
var DefaultMigrations = NewMigrationRegistry()
