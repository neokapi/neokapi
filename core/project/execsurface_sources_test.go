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

// A step that hands a program code as a string is reported with its form,
// by the shape of the invocation: a program, then a flag that takes code. An
// unknown program with such a flag is reported too; a script named as a
// file is not.
func TestExecSite_InlineCode(t *testing.T) {
	site := func(command string, args ...string) ExecSite {
		cfg := map[string]any{"command": command}
		if len(args) > 0 {
			list := make([]any, len(args))
			for i, a := range args {
				list[i] = a
			}
			cfg["args"] = list
		}
		return ExecSite{Name: "external-command", config: cfg}
	}
	for _, tt := range []struct {
		site ExecSite
		want string
	}{
		{site("sh", "-c", "cd scripts && ./hidden.sh"), "sh -c"},
		{site("/bin/bash", "-c", "echo"), "bash -c"},
		{site("cmd.exe", "/C", "dir"), "cmd /C"},
		{site("pwsh", "-Command", "Get-Item"), "pwsh -Command"},
		{site("node", "-e", "require('./lib/x.js')"), "node -e"},
		{site("node", "--eval=require('./lib/x.js')"), "node --eval"},
		{site("node", "-p", "1+1"), "node -p"},
		{site("bun", "--print", "1"), "bun --print"},
		{site("deno", "eval", "console.log(1)"), "deno eval"},
		{site("python3", "-c", "import x"), "python3 -c"},
		{site("perl", "-E", "say 1"), "perl -E"},
		{site("ruby", "-e", "puts 1"), "ruby -e"},
		{site("ruby", "-r", "./lib/x", "tools/y.rb"), "ruby -r"},
		{site("php", "-r", "echo 1;"), "php -r"},
		{site("/usr/bin/env", "node", "-e", "1"), "node -e"},
		{site("my-tool", "-c", "config.yml"), "my-tool -c"},
		{site("some-runner", "-e", "x"), "some-runner -e"},
		{site("node", "tools/x.js"), ""},
		{site("python", "tools/y.py", "--verbose"), ""},
		{site("./tools/run.sh"), ""},
		{site("my-formatter", "-p", "8080"), ""},
	} {
		assert.Equal(t, tt.want, tt.site.InlineCode(), "%v", tt.site.config)
	}
	assert.Empty(t, ExecSite{Name: "script", config: map[string]any{"code": "x"}}.InlineCode())
}
