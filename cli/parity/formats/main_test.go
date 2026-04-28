//go:build parity

package formats

import (
	"fmt"
	"os"
	"testing"

	"github.com/neokapi/neokapi/cli/parity"
)

func TestMain(m *testing.M) {
	code := m.Run()
	parity.ShutdownBridgeDaemon()
	if err := parity.FlushReport(); err != nil {
		fmt.Fprintf(os.Stderr, "parity: flush report: %v\n", err)
	}
	os.Exit(code)
}
