package config

import (
	"errors"
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrationRegistry_SingleStep(t *testing.T) {
	t.Parallel()
	reg := NewMigrationRegistry()
	err := reg.Register(Migration{
		Kind:        FormatConfigKind("html"),
		FromVersion: 1,
		ToVersion:   2,
		Migrate: func(spec map[string]any) (map[string]any, error) {
			result := make(map[string]any)
			maps.Copy(result, spec)
			if pw, ok := result["preserveWhitespace"]; ok {
				delete(result, "preserveWhitespace")
				result["whitespace"] = map[string]any{"preserve": pw}
			}
			return result, nil
		},
	})
	require.NoError(t, err)

	env := &Envelope{
		APIVersion: "v1",
		Kind:       FormatConfigKind("html"),
		Spec:       map[string]any{"preserveWhitespace": true, "useCodeFinder": false},
	}

	err = reg.Upgrade(env)
	require.NoError(t, err)
	assert.Equal(t, "v2", env.APIVersion)
	assert.Nil(t, env.Spec["preserveWhitespace"])
	ws, ok := env.Spec["whitespace"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, ws["preserve"])
	assert.Equal(t, false, env.Spec["useCodeFinder"])
}

func TestMigrationRegistry_MultiStep(t *testing.T) {
	t.Parallel()
	reg := NewMigrationRegistry()
	require.NoError(t, reg.Register(Migration{
		Kind:        FormatConfigKind("json"),
		FromVersion: 1,
		ToVersion:   2,
		Migrate: func(spec map[string]any) (map[string]any, error) {
			spec["migrated_v1_to_v2"] = true
			return spec, nil
		},
	}))
	require.NoError(t, reg.Register(Migration{
		Kind:        FormatConfigKind("json"),
		FromVersion: 2,
		ToVersion:   3,
		Migrate: func(spec map[string]any) (map[string]any, error) {
			spec["migrated_v2_to_v3"] = true
			return spec, nil
		},
	}))

	env := &Envelope{
		APIVersion: "v1",
		Kind:       FormatConfigKind("json"),
		Spec:       map[string]any{"original": true},
	}

	err := reg.Upgrade(env)
	require.NoError(t, err)
	assert.Equal(t, "v3", env.APIVersion)
	assert.Equal(t, true, env.Spec["original"])
	assert.Equal(t, true, env.Spec["migrated_v1_to_v2"])
	assert.Equal(t, true, env.Spec["migrated_v2_to_v3"])
}

func TestMigrationRegistry_AlreadyLatest(t *testing.T) {
	t.Parallel()
	reg := NewMigrationRegistry()
	require.NoError(t, reg.Register(Migration{
		Kind:        FormatConfigKind("html"),
		FromVersion: 1,
		ToVersion:   2,
		Migrate: func(spec map[string]any) (map[string]any, error) {
			return spec, nil
		},
	}))

	env := &Envelope{
		APIVersion: "v2",
		Kind:       FormatConfigKind("html"),
		Spec:       map[string]any{"data": true},
	}

	err := reg.Upgrade(env)
	require.NoError(t, err)
	assert.Equal(t, "v2", env.APIVersion)
	assert.Equal(t, true, env.Spec["data"])
}

func TestMigrationRegistry_NoMigrations(t *testing.T) {
	t.Parallel()
	reg := NewMigrationRegistry()
	env := &Envelope{
		APIVersion: "v1",
		Kind:       FormatConfigKind("html"),
		Spec:       map[string]any{},
	}
	err := reg.Upgrade(env)
	require.NoError(t, err)
}

func TestMigrationRegistry_NewerThanKnown(t *testing.T) {
	t.Parallel()
	reg := NewMigrationRegistry()
	require.NoError(t, reg.Register(Migration{
		Kind:        FormatConfigKind("html"),
		FromVersion: 1,
		ToVersion:   2,
		Migrate: func(spec map[string]any) (map[string]any, error) {
			return spec, nil
		},
	}))

	env := &Envelope{
		APIVersion: "v3",
		Kind:       FormatConfigKind("html"),
		Spec:       map[string]any{},
	}
	err := reg.Upgrade(env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "newer than supported")
}

func TestMigrationRegistry_MigrationError(t *testing.T) {
	t.Parallel()
	reg := NewMigrationRegistry()
	require.NoError(t, reg.Register(Migration{
		Kind:        FormatConfigKind("html"),
		FromVersion: 1,
		ToVersion:   2,
		Migrate: func(spec map[string]any) (map[string]any, error) {
			return nil, errors.New("migration failed")
		},
	}))

	env := &Envelope{
		APIVersion: "v1",
		Kind:       FormatConfigKind("html"),
		Spec:       map[string]any{},
	}
	err := reg.Upgrade(env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "migration failed")
}

func TestMigrationRegistry_RegisterErrors(t *testing.T) {
	t.Parallel()
	reg := NewMigrationRegistry()

	err := reg.Register(Migration{
		Kind:        "",
		FromVersion: 1,
		ToVersion:   2,
		Migrate:     func(spec map[string]any) (map[string]any, error) { return spec, nil },
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kind is required")

	err = reg.Register(Migration{
		Kind:        FormatConfigKind("html"),
		FromVersion: 0,
		ToVersion:   2,
		Migrate:     func(spec map[string]any) (map[string]any, error) { return spec, nil },
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fromVersion")

	err = reg.Register(Migration{
		Kind:        FormatConfigKind("html"),
		FromVersion: 2,
		ToVersion:   1,
		Migrate:     func(spec map[string]any) (map[string]any, error) { return spec, nil },
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "toVersion")
}

func TestMigrationRegistry_LatestVersion(t *testing.T) {
	t.Parallel()
	reg := NewMigrationRegistry()
	htmlKind := FormatConfigKind("html")
	require.NoError(t, reg.Register(Migration{
		Kind:        htmlKind,
		FromVersion: 1,
		ToVersion:   2,
		Migrate:     func(spec map[string]any) (map[string]any, error) { return spec, nil },
	}))
	require.NoError(t, reg.Register(Migration{
		Kind:        htmlKind,
		FromVersion: 2,
		ToVersion:   3,
		Migrate:     func(spec map[string]any) (map[string]any, error) { return spec, nil },
	}))

	assert.Equal(t, 3, reg.LatestVersion(htmlKind))
	assert.Equal(t, 0, reg.LatestVersion(FormatConfigKind("json")))
}
