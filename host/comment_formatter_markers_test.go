package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/plugin/manifest"
)

// A formatter's markers are the configuration files it loads, so the list the
// sourcecode plugin declares for each formatter is the formatter's own search
// list, taken here from the versions this repository installs. A list shorter
// than the formatter's lets a configuration file kapi never read decide what
// the formatter runs.

// installedPackageFile reads rel inside the installed package name, found
// through the repository's node_modules.
func installedPackageFile(t *testing.T, name, glob string) []byte {
	t.Helper()
	dir, err := filepath.EvalSymlinks(filepath.Join("..", "node_modules", name))
	require.NoError(t, err, "%s is installed by `vp install` at the repository root", name)
	matches, err := filepath.Glob(filepath.Join(dir, glob))
	require.NoError(t, err)
	require.Len(t, matches, 1, "exactly one %s in %s", glob, dir)
	data, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	return data
}

// quotedNames returns the double-quoted names in the array literal that follows
// start in src, in order.
func quotedNames(t *testing.T, src []byte, start string) []string {
	t.Helper()
	block := regexp.MustCompile(regexp.QuoteMeta(start) + `(?s)(.*?)\n\];`).FindSubmatch(src)
	require.NotNil(t, block, "%q is in the installed source", start)
	var names []string
	for _, m := range regexp.MustCompile(`(?:name: )?"([^"]+)"`).FindAllSubmatch(block[1], -1) {
		names = append(names, string(m[1]))
	}
	return names
}

// declaredDetect returns the markers the sourcecode manifest declares for the
// formatter name, for every comment language it writes, and asserts that every
// language declares the same ones.
func declaredDetect(t *testing.T, name string) []manifest.CommentFormatterMarker {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "plugins", "sourcecode", "manifest.json"))
	require.NoError(t, err)
	var m manifest.Manifest
	require.NoError(t, json.Unmarshal(data, &m))
	var found []manifest.CommentFormatterMarker
	languages := 0
	for _, c := range m.Capabilities.Comments {
		if c.Rewrite == nil {
			continue
		}
		for _, f := range c.Rewrite.Formatters {
			if f.Name != name {
				continue
			}
			languages++
			if found == nil {
				found = f.Detect
				continue
			}
			assert.Equal(t, found, f.Detect, "%s declares the same %s markers as every other language", c.Language, name)
		}
	}
	require.NotZero(t, languages, "a comment language declares %s", name)
	return found
}

func fileNames(markers []manifest.CommentFormatterMarker) []string {
	names := make([]string, len(markers))
	for i, m := range markers {
		names[i] = m.File
	}
	return names
}

func TestFormatterMarkersAreTheFormattersSearchLists(t *testing.T) {
	t.Run("prettier", func(t *testing.T) {
		searched := quotedNames(t, installedPackageFile(t, "prettier", "index.mjs"), "var CONFIG_FILES = [")
		declared := declaredDetect(t, "prettier")
		assert.Equal(t, searched, fileNames(declared), "prettier's markers are its CONFIG_FILES, in its order")
		for _, m := range declared {
			switch m.File {
			case "package.json", "package.yaml":
				assert.Equal(t, "prettier", m.Key, "prettier reads %s only for its top-level prettier key", m.File)
			default:
				assert.Empty(t, m.Key)
			}
			assert.Empty(t, m.Contains, "prettier loads %s whatever it holds", m.File)
		}
	})

	t.Run("oxfmt through vite-plus", func(t *testing.T) {
		viteConfigs := quotedNames(t, installedPackageFile(t, "vite-plus", filepath.Join("dist", "constants-*.js")), "const VITE_CONFIG_FILES = [")
		declared := declaredDetect(t, "oxfmt")
		assert.Equal(t, append([]string{".oxfmtrc.json", ".oxfmtrc.jsonc"}, viteConfigs...), fileNames(declared),
			"oxfmt's markers are its own configuration files, then the vite configurations vite-plus loads, in its order")
		for _, m := range declared {
			assert.Empty(t, m.Key)
		}
	})
}

func TestHasTopLevelKey(t *testing.T) {
	for _, tc := range []struct {
		name, file, body string
		want             bool
	}{
		{"a top-level object", "package.json", `{"prettier": {}}`, true},
		{"a shared configuration's name", "package.json", `{"prettier": "@acme/prettier-config"}`, true},
		{"only a dependency", "package.json", `{"devDependencies": {"prettier": "3.9.5"}}`, false},
		{"a null value", "package.json", `{"prettier": null}`, false},
		{"an empty string", "package.json", `{"prettier": ""}`, false},
		{"YAML", "package.yaml", "name: p\nprettier:\n  semi: false\n", true},
		{"YAML with the key nested", "package.yaml", "devDependencies:\n  prettier: 3.9.5\n", false},
		{"a document that does not parse", "package.json", `{"prettier": `, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, hasTopLevelKey(tc.file, []byte(tc.body), "prettier"))
		})
	}
}

func TestFormatterEnvKeepsOnlyAbsolutePathEntries(t *testing.T) {
	sep := string(os.PathListSeparator)
	abs := filepath.Join(string(filepath.Separator), "usr", "bin")
	env := formatterEnv([]string{"HOME=/h", "PATH=bin" + sep + "." + sep + sep + abs, "NODE_PATH=rel"})
	assert.Equal(t, []string{"HOME=/h", "PATH=" + abs, "NODE_PATH=rel"}, env)
}

func TestFormatterRuntimeOf(t *testing.T) {
	skipShellFixture(t)
	dir := t.TempDir()
	write := func(name, body string, mode os.FileMode) string {
		t.Helper()
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(body), mode))
		return path
	}
	pathDir := t.TempDir()
	onPath := filepath.Join(pathDir, "node")
	require.NoError(t, os.WriteFile(onPath, []byte("#!/bin/sh\n"), 0o755))
	t.Setenv("PATH", "."+string(os.PathListSeparator)+pathDir)

	shim := write("oxfmt", "#!/bin/sh\nbasedir=$(dirname \"$0\")\nif [ -x \"$basedir/node\" ]; then\n  exec \"$basedir/node\"  \"$basedir/../vite-plus/bin/oxfmt\" \"$@\"\nelse\n  exec node  \"$basedir/../vite-plus/bin/oxfmt\" \"$@\"\nfi\n", 0o755)
	assert.Equal(t, formatterRuntime{interpreter: onPath, script: filepath.Join(filepath.Dir(dir), "vite-plus", "bin", "oxfmt")}, formatterRuntimeOf(shim),
		"a shim with no node beside it runs the node on PATH")

	local := write("node", "#!/bin/sh\n", 0o755)
	assert.Equal(t, local, formatterRuntimeOf(shim).interpreter, "a shim runs the executable node beside it")

	assert.Equal(t, formatterRuntime{interpreter: onPath}, formatterRuntimeOf(write("prettier", "#!/usr/bin/env node\nrequire('x')\n", 0o755)))
	assert.Equal(t, formatterRuntime{interpreter: "/bin/sh"}, formatterRuntimeOf(write("fmt", "#!/bin/sh\nexec x\n", 0o755)))
	assert.Equal(t, formatterRuntime{}, formatterRuntimeOf(write("native", "\x7fELF\x02\x01", 0o755)))
}
