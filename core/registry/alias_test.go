package registry

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegisterAlias_ReaderResolves checks that NewReader resolves an
// alias to the canonical format's factory.
func TestRegisterAlias_ReaderResolves(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "yaml", "YAML",
		[]string{"application/yaml"}, []string{".yaml"})
	reg.RegisterAlias("yaml-catalog", "yaml")

	// Canonical id resolves.
	r, err := reg.NewReader("yaml")
	require.NoError(t, err)
	assert.Equal(t, "yaml", r.Name())

	// Alias resolves to the same canonical factory.
	ra, err := reg.NewReader("yaml-catalog")
	require.NoError(t, err)
	assert.Equal(t, "yaml", ra.Name(), "--format yaml-catalog must resolve to the yaml reader")
}

func TestRegisterAlias_WriterResolves(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "yaml", "YAML",
		[]string{"application/yaml"}, []string{".yaml"})
	reg.RegisterWriter("yaml", func() format.DataFormatWriter { return newStubWriter("yaml") })
	reg.RegisterAlias("yaml-catalog", "yaml")

	w, err := reg.NewWriter("yaml-catalog")
	require.NoError(t, err)
	assert.Equal(t, "yaml", w.Name(), "--format yaml-catalog must resolve to the yaml writer")
}

func TestRegisterAlias_HasReaderHasWriter(t *testing.T) {
	reg := NewFormatRegistry()
	regStub(reg, "yaml")
	reg.RegisterWriter("yaml", func() format.DataFormatWriter { return newStubWriter("yaml") })
	reg.RegisterAlias("yaml-catalog", "yaml")

	assert.True(t, reg.HasReader("yaml"))
	assert.True(t, reg.HasReader("yaml-catalog"), "alias should report a reader")
	assert.True(t, reg.HasWriter("yaml"))
	assert.True(t, reg.HasWriter("yaml-catalog"), "alias should report a writer")

	assert.False(t, reg.HasReader("nope"))
}

func TestRegisterAlias_ResolveReaderWriter(t *testing.T) {
	reg := NewFormatRegistry()
	regStub(reg, "yaml")
	reg.RegisterWriter("yaml", func() format.DataFormatWriter { return newStubWriter("yaml") })
	reg.RegisterAlias("yaml-catalog", "yaml")

	// SubfilterResolver entry points also resolve aliases.
	r, err := reg.ResolveReader("yaml-catalog")
	require.NoError(t, err)
	assert.Equal(t, "yaml", r.Name())

	w, err := reg.ResolveWriter("yaml-catalog")
	require.NoError(t, err)
	assert.Equal(t, "yaml", w.Name())
}

// TestRegisterAlias_NotListed verifies the alias never appears in the
// format listing or detection — only the canonical id does. A user
// searching "yaml" finds the format; the alias stays an implementation
// detail of name resolution.
func TestRegisterAlias_NotListed(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "yaml", "YAML",
		[]string{"application/yaml"}, []string{".yaml"})
	reg.RegisterAlias("yaml-catalog", "yaml")

	infos := reg.FormatInfos()
	names := map[string]bool{}
	for _, info := range infos {
		names[string(info.Name)] = true
	}
	assert.True(t, names["yaml"], "yaml must be listed")
	assert.False(t, names["yaml-catalog"], "the alias must not appear in the format listing")

	// FormatInfo for the alias is nil (no metadata entry).
	assert.Nil(t, reg.FormatInfo("yaml-catalog"))
	assert.NotNil(t, reg.FormatInfo("yaml"))
}

// TestRegisterAlias_DetectionReturnsCanonical verifies that detection
// by extension / MIME returns the canonical id, never the alias —
// because the alias registers no signature.
func TestRegisterAlias_DetectionReturnsCanonical(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "yaml", "YAML",
		[]string{"application/yaml"}, []string{".yaml"})
	reg.RegisterAlias("yaml-catalog", "yaml")

	byExt, err := reg.DetectByExtension(".yaml")
	require.NoError(t, err)
	assert.Equal(t, FormatID("yaml"), byExt)

	byMime := reg.ResolveFormat("application/yaml")
	assert.Equal(t, FormatID("yaml"), byMime)
}

func TestRegisterAlias_AliasTarget(t *testing.T) {
	reg := NewFormatRegistry()
	regStub(reg, "yaml")
	reg.RegisterAlias("yaml-catalog", "yaml")

	target, ok := reg.AliasTarget("yaml-catalog")
	require.True(t, ok)
	assert.Equal(t, FormatID("yaml"), target)

	_, ok = reg.AliasTarget("yaml")
	assert.False(t, ok, "canonical id is not itself an alias")
}

// TestRegisterAlias_SelfIsNoop verifies registering an alias equal to
// the canonical id (or an empty alias) does nothing.
func TestRegisterAlias_SelfIsNoop(t *testing.T) {
	reg := NewFormatRegistry()
	regStub(reg, "yaml")
	reg.RegisterAlias("yaml", "yaml")
	reg.RegisterAlias("", "yaml")

	_, ok := reg.AliasTarget("yaml")
	assert.False(t, ok)
	_, ok = reg.AliasTarget("")
	assert.False(t, ok)
}

// TestRegisterAlias_UnknownAliasStillUnknown verifies that an alias
// pointing at a canonical id with no registered factory still fails to
// resolve (alias resolution is name-only; it does not synthesize a
// factory).
func TestRegisterAlias_UnknownAliasStillUnknown(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterAlias("yaml-catalog", "yaml") // yaml never registered

	_, err := reg.NewReader("yaml-catalog")
	require.Error(t, err)
}
