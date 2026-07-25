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
	regStubSig(reg, "kbf", "Kapi Bundle Format (KBF)",
		[]string{"application/vnd.neokapi.kbf+json"}, []string{".kbf.json"})
	reg.RegisterAlias("jsx", "kbf")

	// Canonical id resolves.
	r, err := reg.NewReader("kbf")
	require.NoError(t, err)
	assert.Equal(t, "kbf", r.Name())

	// Alias resolves to the same canonical factory.
	ra, err := reg.NewReader("jsx")
	require.NoError(t, err)
	assert.Equal(t, "kbf", ra.Name(), "--format jsx must resolve to the kbf reader")
}

func TestRegisterAlias_WriterResolves(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "kbf", "Kapi Bundle Format (KBF)",
		[]string{"application/vnd.neokapi.kbf+json"}, []string{".kbf.json"})
	reg.RegisterWriter("kbf", func() format.DataFormatWriter { return newStubWriter("kbf") })
	reg.RegisterAlias("jsx", "kbf")

	w, err := reg.NewWriter("jsx")
	require.NoError(t, err)
	assert.Equal(t, "kbf", w.Name(), "--format jsx must resolve to the kbf writer")
}

func TestRegisterAlias_HasReaderHasWriter(t *testing.T) {
	reg := NewFormatRegistry()
	regStub(reg, "kbf")
	reg.RegisterWriter("kbf", func() format.DataFormatWriter { return newStubWriter("kbf") })
	reg.RegisterAlias("jsx", "kbf")

	assert.True(t, reg.HasReader("kbf"))
	assert.True(t, reg.HasReader("jsx"), "alias should report a reader")
	assert.True(t, reg.HasWriter("kbf"))
	assert.True(t, reg.HasWriter("jsx"), "alias should report a writer")

	assert.False(t, reg.HasReader("nope"))
}

func TestRegisterAlias_ResolveReaderWriter(t *testing.T) {
	reg := NewFormatRegistry()
	regStub(reg, "kbf")
	reg.RegisterWriter("kbf", func() format.DataFormatWriter { return newStubWriter("kbf") })
	reg.RegisterAlias("jsx", "kbf")

	// SubfilterResolver entry points also resolve aliases.
	r, err := reg.ResolveReader("jsx")
	require.NoError(t, err)
	assert.Equal(t, "kbf", r.Name())

	w, err := reg.ResolveWriter("jsx")
	require.NoError(t, err)
	assert.Equal(t, "kbf", w.Name())
}

// TestRegisterAlias_NotListed verifies the alias never appears in the
// format listing or detection — only the canonical id does. A user
// searching "kbf" finds the format; the alias stays an implementation
// detail of name resolution.
func TestRegisterAlias_NotListed(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "kbf", "Kapi Bundle Format (KBF)",
		[]string{"application/vnd.neokapi.kbf+json"}, []string{".kbf.json"})
	reg.RegisterAlias("jsx", "kbf")

	infos := reg.FormatInfos()
	names := map[string]bool{}
	for _, info := range infos {
		names[string(info.Name)] = true
	}
	assert.True(t, names["kbf"], "kbf must be listed")
	assert.False(t, names["jsx"], "the alias must not appear in the format listing")

	// FormatInfo for the alias is nil (no metadata entry).
	assert.Nil(t, reg.FormatInfo("jsx"))
	assert.NotNil(t, reg.FormatInfo("kbf"))
}

// TestRegisterAlias_DetectionReturnsCanonical verifies that detection
// by extension / MIME returns the canonical id, never the alias —
// because the alias registers no signature.
func TestRegisterAlias_DetectionReturnsCanonical(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "kbf", "Kapi Bundle Format (KBF)",
		[]string{"application/vnd.neokapi.kbf+json"}, []string{".kbf.json"})
	reg.RegisterAlias("jsx", "kbf")

	byExt, err := reg.DetectByExtension(".kbf.json")
	require.NoError(t, err)
	assert.Equal(t, FormatID("kbf"), byExt)

	byMime := reg.ResolveFormat("application/vnd.neokapi.kbf+json")
	assert.Equal(t, FormatID("kbf"), byMime)
}

func TestRegisterAlias_AliasTarget(t *testing.T) {
	reg := NewFormatRegistry()
	regStub(reg, "kbf")
	reg.RegisterAlias("jsx", "kbf")

	target, ok := reg.AliasTarget("jsx")
	require.True(t, ok)
	assert.Equal(t, FormatID("kbf"), target)

	_, ok = reg.AliasTarget("kbf")
	assert.False(t, ok, "canonical id is not itself an alias")
}

// TestRegisterAlias_SelfIsNoop verifies registering an alias equal to
// the canonical id (or an empty alias) does nothing.
func TestRegisterAlias_SelfIsNoop(t *testing.T) {
	reg := NewFormatRegistry()
	regStub(reg, "kbf")
	reg.RegisterAlias("kbf", "kbf")
	reg.RegisterAlias("", "kbf")

	_, ok := reg.AliasTarget("kbf")
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
	reg.RegisterAlias("jsx", "kbf") // kbf never registered

	_, err := reg.NewReader("jsx")
	require.Error(t, err)
}
