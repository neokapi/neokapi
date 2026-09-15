package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A formatter's exec site digests what would run: the same facts give the same
// digest, and a changed executable or configuration file a different one.
func TestFormatterExecSiteDigest(t *testing.T) {
	site := func(executable, config string) ExecSite {
		return FormatterExecSite("/p/.prettierrc", "prettier", []string{"/p/node_modules/.bin/prettier", "--stdin-filepath", "{file}"},
			map[string]any{"executable_sha256": executable, "config_sha256": config})
	}
	digest := func(s ExecSite) string { return ExecSurfaceDigest([]ExecSite{s}) }
	base := digest(site("e1", "c1"))
	assert.Equal(t, base, digest(site("e1", "c1")))
	assert.NotEqual(t, base, digest(site("e2", "c1")), "a changed executable asks again")
	assert.NotEqual(t, base, digest(site("e1", "c2")), "a changed configuration file asks again")
	assert.Equal(t, "formatter", site("e1", "c1").Kind)
	assert.Contains(t, site("e1", "c1").Detail, "/p/node_modules/.bin/prettier --stdin-filepath")
}
