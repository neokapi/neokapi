package cli

import (
	"sync/atomic"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	jsonformat "github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// registerStubEngine registers okf_json, a JSON format from a plugin engine
// named stubengine that claims .json as the built-in json does, and returns a
// count of the readers it built.
func registerStubEngine(t *testing.T, a *App) *atomic.Int32 {
	t.Helper()
	var built atomic.Int32
	a.FormatReg.RegisterReader("okf_json", func() format.DataFormatReader {
		built.Add(1)
		return jsonformat.NewReader()
	}, format.FormatSignature{Extensions: []string{".json"}}, "Stub JSON")
	a.FormatReg.RegisterWriter("okf_json", func() format.DataFormatWriter { return jsonformat.NewWriter() })
	a.FormatReg.RegisterFormatInfo("okf_json", registry.FormatInfo{Source: "stubengine", Extensions: []string{".json"}})
	return &built
}

// `kapi up` reads with the engine the recipe selects (defaults.engine), so
// `--engine` has no work to do on `up`: a person with a second engine
// installed selects it in the recipe.
func TestUpHonoursTheRecipeEngine(t *testing.T) {
	isolate(t)
	for _, tc := range []struct {
		engine string
		want   bool
	}{{"", false}, {"stubengine", true}} {
		t.Run("engine "+tc.engine, func(t *testing.T) {
			recipe, _ := convergeFixture(t, []model.LocaleID{"fr-FR"}, gate.Gate{"translated": {Pct: 100}})
			proj, err := project.Load(recipe)
			require.NoError(t, err)
			for i := range proj.Collections {
				proj.Collections[i].Format = nil // detected, so the engine decides
			}
			proj.Defaults.Engine = tc.engine
			proj.Plugins = map[string]project.PluginSpec{"stubengine": {Version: "1.0.0"}}
			require.NoError(t, project.Save(recipe, proj))

			a := processOnlyApp(t)
			built := registerStubEngine(t, a)
			out, err := runUp(t, a, recipe)
			require.NoError(t, err, out)
			assert.Equal(t, tc.want, built.Load() > 0, "the stub engine's reader read the sources: %d", built.Load())
		})
	}
}
