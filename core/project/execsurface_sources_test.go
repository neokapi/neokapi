package project

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Every field of the recipe either feeds the exec surface or is listed with
// why it declares nothing kapi executes, so a new place a step can be
// declared cannot be added without joining the surface or saying why not.
func TestExecSurfaceSources_CoverEveryRecipeField(t *testing.T) {
	census := func(t *testing.T, typ reflect.Type, sources []string, others map[string]bool) {
		t.Helper()
		for f := range typ.Fields() {
			if !f.IsExported() {
				continue
			}
			assert.True(t, slices.Contains(sources, f.Name) || others[f.Name],
				"%s.%s is not classified: add it to the exec surface walk (ExecSurfaceIn) or say why it declares nothing kapi executes", typ.Name(), f.Name)
		}
	}
	nonSources := map[string]bool{}
	for k := range ExecSurfaceNonSources {
		nonSources[k] = true
	}
	census(t, reflect.TypeFor[KapiProject](), ExecSurfaceSources, nonSources)

	// defaults: carries the presets; every other field configures readers,
	// languages and runs, and names no step.
	defaultsOthers := map[string]bool{}
	for f := range reflect.TypeFor[Defaults]().Fields() {
		if !slices.Contains(ExecSurfaceDefaultsSources, f.Name) {
			defaultsOthers[f.Name] = true
		}
	}
	for _, name := range []string{"Flow", "Formats", "Comments", "Extras"} {
		require.True(t, defaultsOthers[name], "defaults.%s is expected to name no step", name)
	}
	// A defaults field that holds a step spec or a tool preset map would be a
	// source: no field other than the presets may have that type.
	for f := range reflect.TypeFor[Defaults]().Fields() {
		if slices.Contains(ExecSurfaceDefaultsSources, f.Name) {
			continue
		}
		assert.NotEqual(t, reflect.TypeFor[map[string]map[string]any](), f.Type, "defaults.%s holds tool presets", f.Name)
	}
}

// A flow file in flows_dir is part of what the project can execute, located
// by its path.
func TestExecSurfaceIn_ReadsTheFlowFiles(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "flows"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "flows", "sneaky.yaml"),
		[]byte("steps:\n  - tool: external-command\n    config:\n      command: ./x.sh\n"), 0o644))
	var p KapiProject
	require.NoError(t, yaml.Unmarshal([]byte("version: v1\nflows_dir: flows\n"), &p))

	assert.Empty(t, ExecSurface(&p), "the recipe alone names no command")
	sites := ExecSurfaceIn(&p, root)
	require.Len(t, sites, 1)
	assert.Equal(t, "flows/sneaky.yaml steps[0]", sites[0].Where)
	assert.Equal(t, "external-command", sites[0].Name)
}
