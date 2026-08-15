package connector

import (
	"testing"

	venueconn "github.com/neokapi/neokapi/core/venue/connector"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
)

func testFormatRegistry(t *testing.T) *registry.FormatRegistry {
	t.Helper()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	return reg
}

func registeredTypes(r *platconn.Registry) []string {
	infos := r.List()
	names := make([]string, 0, len(infos))
	for _, info := range infos {
		names = append(names, info.Name)
	}
	return names
}

// TestRegisterServerHasNoLocalFilesystemConnector is the tenant-isolation
// guard. Connector configs arrive over the workspace connectors API and adding
// one needs only PermManageConnectors, which the built-in developer role
// carries — so any connector the server registry can build, a tenant can build
// with a config of their choosing. A `file` or `git` connector takes a path
// from that config, which is why neither may be constructible here.
func TestRegisterServerHasNoLocalFilesystemConnector(t *testing.T) {
	reg := platconn.NewRegistry()
	RegisterServer(reg, testFormatRegistry(t))

	for _, typ := range []string{"file", "git"} {
		t.Run(typ, func(t *testing.T) {
			assert.False(t, reg.Has(typ),
				"the multi-tenant server registry must not offer %q", typ)

			// Not merely absent from the listing — unconstructible. A config
			// naming a filesystem root must fail, not silently take it.
			c, err := reg.NewConnector(typ, map[string]string{
				"path": "/",
				"repo": "https://example.com/acme/site.git",
			})
			require.Error(t, err)
			assert.Nil(t, c)
		})
	}
}

// TestRegisterServerOffersRemoteAndForge: the fix removes the local
// connectors, and nothing else. Everything the product sells through the
// connectors API must still be there.
func TestRegisterServerOffersRemoteAndForge(t *testing.T) {
	reg := platconn.NewRegistry()
	RegisterServer(reg, testFormatRegistry(t))

	assert.ElementsMatch(t,
		[]string{"forge", "wordpress", "figma", "hubspot"},
		registeredTypes(reg),
		"the server surface is the remote integrations plus forge")
}

// TestRegisterLocalOffersFilesystemConnectors: the single-tenant surface still
// has them, so the split is a boundary rather than a deletion.
func TestRegisterLocalOffersFilesystemConnectors(t *testing.T) {
	reg := platconn.NewRegistry()
	RegisterLocal(reg, testFormatRegistry(t))

	assert.ElementsMatch(t,
		[]string{"file", "git", "forge", "wordpress", "figma", "hubspot"},
		registeredTypes(reg))

	c, err := reg.NewConnector("file", map[string]string{"path": t.TempDir()})
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, venueconn.CategoryFile, c.Category())
}

// TestRegisterRemoteIsFilesystemFree: the desktop app's registry, which is
// also the base RegisterServer builds on.
func TestRegisterRemoteIsFilesystemFree(t *testing.T) {
	reg := platconn.NewRegistry()
	RegisterRemote(reg)

	assert.ElementsMatch(t,
		[]string{"wordpress", "figma", "hubspot"},
		registeredTypes(reg))
	assert.False(t, reg.Has("file"))
	assert.False(t, reg.Has("git"))
	assert.False(t, reg.Has("forge"))
}
