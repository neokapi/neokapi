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
	regStubSig(reg, "klf", "Kapi Localization Format (KLF)",
		[]string{"application/vnd.neokapi.klf+json"}, []string{".klf"})
	reg.RegisterAlias("jsx", "klf")

	// Canonical id resolves.
	r, err := reg.NewReader("klf")
	require.NoError(t, err)
	assert.Equal(t, "klf", r.Name())

	// Alias resolves to the same canonical factory.
	ra, err := reg.NewReader("jsx")
	require.NoError(t, err)
	assert.Equal(t, "klf", ra.Name(), "--format jsx must resolve to the klf reader")
}

func TestRegisterAlias_WriterResolves(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "klf", "Kapi Localization Format (KLF)",
		[]string{"application/vnd.neokapi.klf+json"}, []string{".klf"})
	reg.RegisterWriter("klf", func() format.DataFormatWriter { return newStubWriter("klf") })
	reg.RegisterAlias("jsx", "klf")

	w, err := reg.NewWriter("jsx")
	require.NoError(t, err)
	assert.Equal(t, "klf", w.Name(), "--format jsx must resolve to the klf writer")
}

func TestRegisterAlias_HasReaderHasWriter(t *testing.T) {
	reg := NewFormatRegistry()
	regStub(reg, "klf")
	reg.RegisterWriter("klf", func() format.DataFormatWriter { return newStubWriter("klf") })
	reg.RegisterAlias("jsx", "klf")

	assert.True(t, reg.HasReader("klf"))
	assert.True(t, reg.HasReader("jsx"), "alias should report a reader")
	assert.True(t, reg.HasWriter("klf"))
	assert.True(t, reg.HasWriter("jsx"), "alias should report a writer")

	assert.False(t, reg.HasReader("nope"))
}

func TestRegisterAlias_ResolveReaderWriter(t *testing.T) {
	reg := NewFormatRegistry()
	regStub(reg, "klf")
	reg.RegisterWriter("klf", func() format.DataFormatWriter { return newStubWriter("klf") })
	reg.RegisterAlias("jsx", "klf")

	// SubfilterResolver entry points also resolve aliases.
	r, err := reg.ResolveReader("jsx")
	require.NoError(t, err)
	assert.Equal(t, "klf", r.Name())

	w, err := reg.ResolveWriter("jsx")
	require.NoError(t, err)
	assert.Equal(t, "klf", w.Name())
}

// TestRegisterAlias_NotListed verifies the alias never appears in the
// format listing or detection — only the canonical id does. A user
// searching "klf" finds the format; the alias stays an implementation
// detail of name resolution.
func TestRegisterAlias_NotListed(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "klf", "Kapi Localization Format (KLF)",
		[]string{"application/vnd.neokapi.klf+json"}, []string{".klf"})
	reg.RegisterAlias("jsx", "klf")

	infos := reg.FormatInfos()
	names := map[string]bool{}
	for _, info := range infos {
		names[string(info.Name)] = true
	}
	assert.True(t, names["klf"], "klf must be listed")
	assert.False(t, names["jsx"], "the alias must not appear in the format listing")

	// FormatInfo for the alias is nil (no metadata entry).
	assert.Nil(t, reg.FormatInfo("jsx"))
	assert.NotNil(t, reg.FormatInfo("klf"))
}

// TestRegisterAlias_DetectionReturnsCanonical verifies that detection
// by extension / MIME returns the canonical id, never the alias —
// because the alias registers no signature.
func TestRegisterAlias_DetectionReturnsCanonical(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "klf", "Kapi Localization Format (KLF)",
		[]string{"application/vnd.neokapi.klf+json"}, []string{".klf"})
	reg.RegisterAlias("jsx", "klf")

	byExt, err := reg.DetectByExtension(".klf")
	require.NoError(t, err)
	assert.Equal(t, FormatID("klf"), byExt)

	byMime := reg.ResolveFormat("application/vnd.neokapi.klf+json")
	assert.Equal(t, FormatID("klf"), byMime)
}

func TestRegisterAlias_AliasTarget(t *testing.T) {
	reg := NewFormatRegistry()
	regStub(reg, "klf")
	reg.RegisterAlias("jsx", "klf")

	target, ok := reg.AliasTarget("jsx")
	require.True(t, ok)
	assert.Equal(t, FormatID("klf"), target)

	_, ok = reg.AliasTarget("klf")
	assert.False(t, ok, "canonical id is not itself an alias")
}

// TestRegisterAlias_SelfIsNoop verifies registering an alias equal to
// the canonical id (or an empty alias) does nothing.
func TestRegisterAlias_SelfIsNoop(t *testing.T) {
	reg := NewFormatRegistry()
	regStub(reg, "klf")
	reg.RegisterAlias("klf", "klf")
	reg.RegisterAlias("", "klf")

	_, ok := reg.AliasTarget("klf")
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
	reg.RegisterAlias("jsx", "klf") // klf never registered

	_, err := reg.NewReader("jsx")
	require.Error(t, err)
}
