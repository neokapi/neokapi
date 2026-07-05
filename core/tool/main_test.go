package tool

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// The immutability hash backstop ships off by default (production runs
	// skip the per-block hashing); tests keep it on so a handler mutating
	// content in place fails loudly. TestImmutabilityGuard and friends rely
	// on this.
	EnforceImmutability = true
	goleak.VerifyTestMain(m)
}
