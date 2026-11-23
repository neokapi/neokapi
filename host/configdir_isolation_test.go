package host

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/host/credentials"
)

// A test that sets no KAPI_CONFIG_DIR must not reach the developer's kapi
// configuration: the config root, and providers.json under it, resolve under
// the system temporary directory, where this test binary keeps its own tree.
func TestConfigDirInATestBinaryIsNotTheDevelopersOwn(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", "")

	for name, p := range map[string]string{
		"ConfigDir":               ConfigDir(),
		"credentials.DefaultPath": credentials.DefaultPath(),
	} {
		assert.True(t, strings.HasPrefix(p, os.TempDir()), "%s() = %s", name, p)
	}
}
