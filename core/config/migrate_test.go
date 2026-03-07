package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrationRegistry_SingleStep(t *testing.T) {
	reg := NewMigrationRegistry()
	err := reg.Register(Migration{
		FromVersion: "gokapi/html-v1",
		ToVersion:   "gokapi/html-v2",
		Migrate: func(spec map[string]any) (map[string]any, error) {
			// v2 renames "preserveWhitespace" to "whitespace.preserve"
			result := make(map[string]any)
			for k, v := range spec {
				result[k] = v
			}
			if pw, ok := result["preserveWhitespace"]; ok {
				delete(result, "preserveWhitespace")
				result["whitespace"] = map[string]any{"preserve": pw}
			}
			return result, nil
		},
	})
	require.NoError(t, err)

	env := &Envelope{
		APIVersion: "gokapi/html-v1",
		Spec:       map[string]any{"preserveWhitespace": true, "useCodeFinder": false},
	}

	err = reg.Upgrade(env)
	require.NoError(t, err)
	assert.Equal(t, "gokapi/html-v2", env.APIVersion)
	assert.Nil(t, env.Spec["preserveWhitespace"])
	ws, ok := env.Spec["whitespace"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, ws["preserve"])
	assert.Equal(t, false, env.Spec["useCodeFinder"])
}

func TestMigrationRegistry_MultiStep(t *testing.T) {
	reg := NewMigrationRegistry()
	require.NoError(t, reg.Register(Migration{
		FromVersion: "gokapi/json-v1",
		ToVersion:   "gokapi/json-v2",
		Migrate: func(spec map[string]any) (map[string]any, error) {
			spec["migrated_v1_to_v2"] = true
			return spec, nil
		},
	}))
	require.NoError(t, reg.Register(Migration{
		FromVersion: "gokapi/json-v2",
		ToVersion:   "gokapi/json-v3",
		Migrate: func(spec map[string]any) (map[string]any, error) {
			spec["migrated_v2_to_v3"] = true
			return spec, nil
		},
	}))

	env := &Envelope{
		APIVersion: "gokapi/json-v1",
		Spec:       map[string]any{"original": true},
	}

	err := reg.Upgrade(env)
	require.NoError(t, err)
	assert.Equal(t, "gokapi/json-v3", env.APIVersion)
	assert.Equal(t, true, env.Spec["original"])
	assert.Equal(t, true, env.Spec["migrated_v1_to_v2"])
	assert.Equal(t, true, env.Spec["migrated_v2_to_v3"])
}

func TestMigrationRegistry_AlreadyLatest(t *testing.T) {
	reg := NewMigrationRegistry()
	require.NoError(t, reg.Register(Migration{
		FromVersion: "gokapi/html-v1",
		ToVersion:   "gokapi/html-v2",
		Migrate: func(spec map[string]any) (map[string]any, error) {
			return spec, nil
		},
	}))

	env := &Envelope{
		APIVersion: "gokapi/html-v2",
		Spec:       map[string]any{"data": true},
	}

	err := reg.Upgrade(env)
	require.NoError(t, err)
	assert.Equal(t, "gokapi/html-v2", env.APIVersion)
	assert.Equal(t, true, env.Spec["data"])
}

func TestMigrationRegistry_NoMigrations(t *testing.T) {
	reg := NewMigrationRegistry()
	env := &Envelope{
		APIVersion: "gokapi/html-v1",
		Spec:       map[string]any{},
	}
	err := reg.Upgrade(env)
	require.NoError(t, err)
}

func TestMigrationRegistry_NewerThanKnown(t *testing.T) {
	reg := NewMigrationRegistry()
	require.NoError(t, reg.Register(Migration{
		FromVersion: "gokapi/html-v1",
		ToVersion:   "gokapi/html-v2",
		Migrate: func(spec map[string]any) (map[string]any, error) {
			return spec, nil
		},
	}))

	env := &Envelope{
		APIVersion: "gokapi/html-v3",
		Spec:       map[string]any{},
	}
	err := reg.Upgrade(env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "newer than supported")
}

func TestMigrationRegistry_MigrationError(t *testing.T) {
	reg := NewMigrationRegistry()
	require.NoError(t, reg.Register(Migration{
		FromVersion: "gokapi/html-v1",
		ToVersion:   "gokapi/html-v2",
		Migrate: func(spec map[string]any) (map[string]any, error) {
			return nil, fmt.Errorf("migration failed")
		},
	}))

	env := &Envelope{
		APIVersion: "gokapi/html-v1",
		Spec:       map[string]any{},
	}
	err := reg.Upgrade(env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "migration failed")
}

func TestMigrationRegistry_RegisterErrors(t *testing.T) {
	reg := NewMigrationRegistry()

	err := reg.Register(Migration{
		FromVersion: "invalid",
		ToVersion:   "gokapi/html-v2",
		Migrate:     func(spec map[string]any) (map[string]any, error) { return spec, nil },
	})
	assert.Error(t, err)

	err = reg.Register(Migration{
		FromVersion: "gokapi/html-v1",
		ToVersion:   "invalid",
		Migrate:     func(spec map[string]any) (map[string]any, error) { return spec, nil },
	})
	assert.Error(t, err)

	err = reg.Register(Migration{
		FromVersion: "gokapi/html-v1",
		ToVersion:   "gokapi/json-v2",
		Migrate:     func(spec map[string]any) (map[string]any, error) { return spec, nil },
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resource mismatch")
}

func TestMigrationRegistry_LatestVersion(t *testing.T) {
	reg := NewMigrationRegistry()
	require.NoError(t, reg.Register(Migration{
		FromVersion: "gokapi/html-v1",
		ToVersion:   "gokapi/html-v2",
		Migrate:     func(spec map[string]any) (map[string]any, error) { return spec, nil },
	}))
	require.NoError(t, reg.Register(Migration{
		FromVersion: "gokapi/html-v2",
		ToVersion:   "gokapi/html-v3",
		Migrate:     func(spec map[string]any) (map[string]any, error) { return spec, nil },
	}))

	assert.Equal(t, 3, reg.LatestVersion("gokapi/html"))
	assert.Equal(t, 0, reg.LatestVersion("gokapi/json"))
}
