//go:build parity

package roundtrip_test

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/cli/parity"
)

func TestMain(m *testing.M) {
	code := m.Run()
	parity.ShutdownBridgeDaemon()
	os.Exit(code)
}
