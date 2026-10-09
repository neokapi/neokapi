package host

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rootResolverFiles are the sources that turn an environment variable into a
// root kapi reads or writes under. A KAPI_ or XDG_ variable one of them reads
// belongs in RootEnvVars, or a launcher that forwards the list by name would
// start a kapi that keeps its stores somewhere else.
var rootResolverFiles = []string{
	"datadir.go",
	"resource.go",
	"kpzcache.go",
	"modelassets.go",
	"config/config.go",
	"credentials/store.go",
	"pluginhost/discover.go",
	"pluginhost/install.go",
	"pluginhost/cache.go",
	"pluginhost/registry/cache.go",
	"../core/project/recipepath.go",
}

var envLiteral = regexp.MustCompile(`"((?:KAPI|XDG)_[A-Z0-9_]+)"`)

// Every variable a root resolver reads is in the list, so adding one to a
// resolver without naming it here fails here rather than in a Codex session
// that silently kept two workspaces.
func TestRootEnvVars_CoversEveryRootResolver(t *testing.T) {
	for _, rel := range rootResolverFiles {
		src, err := os.ReadFile(filepath.Join(".", filepath.FromSlash(rel)))
		require.NoError(t, err, rel)
		for _, m := range envLiteral.FindAllStringSubmatch(string(src), -1) {
			name := m[1]
			if name == "KAPI_REGISTRY_URL" || name == "KAPI_UPDATE_CHANNEL" {
				// Settings a resolver file happens to read: where the registry
				// is and which release channel to follow, not where kapi
				// keeps anything.
				continue
			}
			assert.Contains(t, RootEnvVars, name, "%s reads %s, which RootEnvVars does not name", rel, name)
		}
	}
}

func TestRootEnvVars_IsUniqueAndSorted(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range RootEnvVars {
		assert.False(t, seen[name], "%s is listed twice", name)
		seen[name] = true
	}
	assert.True(t, slices.Contains(RootEnvVars, EnvDataDir))
	assert.True(t, slices.Contains(RootEnvVars, "KAPI_NO_PROJECT"))
}
