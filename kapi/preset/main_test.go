package preset

import (
	"testing"

	"github.com/neokapi/neokapi/core/dataroot"
	"github.com/neokapi/neokapi/core/devenvtest"
)

// These tests resolve kapi's per-user locations, which a test binary keeps in
// a root of its own under the temporary directory; it is removed once they
// have run.
func TestMain(m *testing.M) { devenvtest.Main(m, dataroot.RemoveTestDir) }
