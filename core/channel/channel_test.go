package channel

import (
	"testing"

	"github.com/neokapi/neokapi/core/version"
)

// A fresh prerelease build with nothing persisted and no env override infers
// beta, pins it, and the choice survives a later update to a final version —
// because the beta channel now also serves finals, so inference alone would
// wrongly flip a beta user to stable.
func TestStickyBetaAcrossFinal(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv(Env, "")
	saved := version.Version
	defer func() { version.Version = saved }()

	version.Version = "1.2.0-rc.1"
	if got := Resolve(); got != "beta" {
		t.Fatalf("fresh prerelease build: Resolve()=%q, want beta", got)
	}
	EnsurePinned()
	if Persisted() != "beta" {
		t.Fatalf("EnsurePinned did not persist beta; Persisted()=%q", Persisted())
	}

	// Update to the final release: version no longer infers beta, but the pin holds.
	version.Version = "1.2.0"
	if got := Resolve(); got != "beta" {
		t.Fatalf("after update to final: Resolve()=%q, want sticky beta", got)
	}
}

func TestStableBuildNoPin(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv(Env, "")
	saved := version.Version
	defer func() { version.Version = saved }()
	version.Version = "1.2.0"

	EnsurePinned()
	if Persisted() != "" {
		t.Fatalf("a stable build must not pin; Persisted()=%q", Persisted())
	}
	if got := Resolve(); got != "stable" {
		t.Fatalf("Resolve()=%q, want stable", got)
	}
}

func TestEnvWinsAndIsNotPersisted(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv(Env, "beta")
	saved := version.Version
	defer func() { version.Version = saved }()
	version.Version = "1.2.0" // stable build, but env forces beta

	if got := Resolve(); got != "beta" {
		t.Fatalf("env override: Resolve()=%q, want beta", got)
	}
	EnsurePinned()
	if Persisted() != "" {
		t.Fatalf("env override must not be persisted; Persisted()=%q", Persisted())
	}
}
