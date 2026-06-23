// Package channel resolves and persists the kapi update channel (stable | beta),
// shared by the CLI and both desktop apps so a machine follows one channel
// consistently. It uses a plain one-line file alongside kapi.yaml — no Viper —
// so the bowrain desktop (which stays Viper-free) can read and write it too.
//
// Resolution precedence: KAPI_UPDATE_CHANNEL env > persisted preference > the
// channel inferred from this build's version (a prerelease defaults to beta).
// The persisted preference makes beta membership sticky: a beta build that later
// updates to a final (non-prerelease) version stays on the fast track instead of
// inferring stable — which matters because the beta channel also carries final
// releases (so beta users never fall behind stable).
package channel

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/version"
)

// Env overrides the channel for every app (CLI + desktop).
const Env = "KAPI_UPDATE_CHANNEL"

// fileName is the one-line channel file kept next to kapi.yaml.
const fileName = "update-channel"

// dir returns the directory holding kapi's user config. Mirrors cli/config's
// resolution (KAPI_CONFIG_DIR, else <UserConfigDir>/kapi) without importing it.
func dir() string {
	if d := strings.TrimSpace(os.Getenv("KAPI_CONFIG_DIR")); d != "" {
		return d
	}
	d, err := os.UserConfigDir()
	if err != nil || d == "" {
		d = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(d, "kapi")
}

func filePath() string { return filepath.Join(dir(), fileName) }

// Persisted returns the channel saved on disk, or "" if none/unreadable.
func Persisted() string {
	b, err := os.ReadFile(filePath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Resolve returns the active channel (env > persisted > inferred-from-build).
func Resolve() string {
	if c := strings.TrimSpace(os.Getenv(Env)); c != "" {
		return c
	}
	if c := Persisted(); c != "" {
		return c
	}
	return version.Channel()
}

// Pin persists c (best-effort).
func Pin(c string) error {
	c = strings.TrimSpace(c)
	if c == "" {
		return nil
	}
	if err := os.MkdirAll(dir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filePath(), []byte(c+"\n"), 0o644)
}

// EnsurePinned makes beta membership sticky: the first time a prerelease build
// runs with no env override and nothing persisted, it pins "beta" so the choice
// survives a later update to a final version. No-op otherwise. Best-effort.
func EnsurePinned() {
	if strings.TrimSpace(os.Getenv(Env)) != "" {
		return // explicit, ephemeral override — don't persist it
	}
	if Persisted() != "" {
		return // already pinned
	}
	if !version.IsPrerelease() {
		return // a stable build has nothing to pin
	}
	_ = Pin("beta")
}
