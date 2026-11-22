package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
)

// TestUp_RecycleOnlyFillsExactMatchesInOnePass: `kapi up --recycle-only`
// runs the recycle-only flow in place of the project's own, once, filling the
// one block the content memory answers and drafting nothing else.
func TestUp_RecycleOnlyFillsExactMatchesInOnePass(t *testing.T) {
	t.Setenv("KAPI_ACTOR", "person")
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"fr-FR"}, gate.Gate{"translated": {Pct: 100}})
	seedProjectMemory(t, root, memory.Entry{ID: "hello", Variants: map[model.LocaleID][]model.Run{
		"en-US": {model.TextR("Hello, world.")},
		"fr-FR": {model.TextR("Bonjour le monde.")},
	}})

	out, err := runUp(t, a, recipe, "--recycle-only")
	require.NoError(t, err, out)
	assert.Contains(t, out, "(recycled 1 · AI 0)")
	assert.Contains(t, out, `Ran flow "recycle" over 1 locale(s) in 1 pass.`)
}
