package doxygen

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
)

func TestMain(m *testing.M) { os.Exit(bridgetest.Run(m)) }
