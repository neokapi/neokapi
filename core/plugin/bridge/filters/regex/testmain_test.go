package regex

import (
	"os"
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

func TestMain(m *testing.M) { os.Exit(bridgetest.Run(m)) }
