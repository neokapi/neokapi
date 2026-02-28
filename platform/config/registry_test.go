package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistriesDefault(t *testing.T) {
	cfg := NewAppConfig()
	regs := cfg.Registries()
	require.Len(t, regs, 1)
	assert.Equal(t, "default", regs[0].Name)
	assert.Equal(t, DefaultRegistryURL, regs[0].URL)
}

func TestRegistriesFromViper(t *testing.T) {
	cfg := NewAppConfig()
	cfg.v.Set("registries", []map[string]any{
		{"name": "official", "url": "https://example.com/official.json"},
		{"name": "company", "url": "https://example.com/company.json"},
	})

	regs := cfg.Registries()
	require.Len(t, regs, 2)
	assert.Equal(t, "official", regs[0].Name)
	assert.Equal(t, "https://example.com/official.json", regs[0].URL)
	assert.Equal(t, "company", regs[1].Name)
	assert.Equal(t, "https://example.com/company.json", regs[1].URL)
}

func TestRegistriesFallbackToPluginsRegistry(t *testing.T) {
	cfg := NewAppConfig()
	cfg.v.Set("plugins.registry", "https://custom.example.com/plugins.json")

	regs := cfg.Registries()
	require.Len(t, regs, 1)
	assert.Equal(t, "default", regs[0].Name)
	assert.Equal(t, "https://custom.example.com/plugins.json", regs[0].URL)
}

func TestAddGlobalRegistry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", dir)

	require.NoError(t, AddGlobalRegistry("test", "https://example.com/plugins.json"))

	regs, err := ListGlobalRegistries()
	require.NoError(t, err)
	require.Len(t, regs, 1)
	assert.Equal(t, "test", regs[0].Name)
	assert.Equal(t, "https://example.com/plugins.json", regs[0].URL)
}

func TestAddGlobalRegistryDuplicate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", dir)

	require.NoError(t, AddGlobalRegistry("test", "https://example.com/plugins.json"))
	err := AddGlobalRegistry("test", "https://example.com/other.json")
	assert.ErrorContains(t, err, "already exists")
}

func TestRemoveGlobalRegistry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", dir)

	require.NoError(t, AddGlobalRegistry("a", "https://example.com/a.json"))
	require.NoError(t, AddGlobalRegistry("b", "https://example.com/b.json"))

	require.NoError(t, RemoveGlobalRegistry("a"))

	regs, err := ListGlobalRegistries()
	require.NoError(t, err)
	require.Len(t, regs, 1)
	assert.Equal(t, "b", regs[0].Name)
}

func TestRemoveGlobalRegistryNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", dir)

	err := RemoveGlobalRegistry("nonexistent")
	assert.ErrorContains(t, err, "not found")
}

func TestListGlobalRegistriesEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", dir)

	regs, err := ListGlobalRegistries()
	require.NoError(t, err)
	require.Len(t, regs, 1)
	assert.Equal(t, "default", regs[0].Name)
	assert.Equal(t, DefaultRegistryURL, regs[0].URL)
}
