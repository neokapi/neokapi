package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/neokapi/neokapi/bowrain/core/config"
	"github.com/zalando/go-keyring"
)

// Legacy desktop-only auth scheme (pre-unification). The desktop used to
// persist its own tokens under keychain service "bowrain" (keys
// "access-token"/"refresh-token") plus a bowrain-desktop/auth.json metadata
// file — a stack entirely separate from the shared kapi/kapi-bowrain scheme in
// bowrain/core/config. migrateLegacyDesktopAuth() folds any such credentials
// into the shared store once, so an existing desktop login survives the switch
// and a CLI login and a desktop login become mutually visible.
const (
	legacyKeyringServiceBase = "bowrain"
	legacyAccessTokenKey     = "access-token"
	legacyRefreshTokenKey    = "refresh-token"
)

// legacyKeyringService mirrors the old config-dir-namespaced service name.
func legacyKeyringService() string {
	if dir := os.Getenv("BOWRAIN_DESKTOP_CONFIG_DIR"); dir != "" {
		return legacyKeyringServiceBase + ":" + dir
	}
	return legacyKeyringServiceBase
}

// legacyDesktopAuthFilePath returns the old metadata path.
func legacyDesktopAuthFilePath() string {
	return filepath.Join(desktopConfigDir(), "auth.json")
}

// legacyDesktopAuth is the old on-disk metadata shape.
type legacyDesktopAuth struct {
	ServerURL string    `json:"server_url"`
	Expiry    time.Time `json:"expiry"`
	User      struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	} `json:"user"`
}

// migrateLegacyDesktopAuth performs a one-time migration of desktop credentials
// from the legacy scheme to the shared bowrain/core/config store. It is a no-op
// when the shared store already holds credentials (a CLI login, or a prior
// migration) or when no legacy data exists. Cleanup of the legacy entries is
// best-effort.
func migrateLegacyDesktopAuth() {
	// If the shared store already has metadata, don't touch it — a desktop or
	// CLI login has already populated the unified scheme.
	if _, err := os.Stat(config.AuthFilePath()); err == nil {
		return
	}

	data, err := os.ReadFile(legacyDesktopAuthFilePath())
	if err != nil {
		return
	}
	var legacy legacyDesktopAuth
	if err := json.Unmarshal(data, &legacy); err != nil || legacy.ServerURL == "" {
		return
	}

	access, err := keyring.Get(legacyKeyringService(), legacyAccessTokenKey)
	if err != nil || access == "" {
		return
	}
	refresh, _ := keyring.Get(legacyKeyringService(), legacyRefreshTokenKey)

	if err := config.SaveAuth(config.StoredAuth{
		ServerURL:    legacy.ServerURL,
		AccessToken:  access,
		RefreshToken: refresh,
		Expiry:       legacy.Expiry,
		User: config.StoredUser{
			ID:    legacy.User.ID,
			Email: legacy.User.Email,
			Name:  legacy.User.Name,
		},
	}); err != nil {
		return
	}

	// Best-effort cleanup of the legacy scheme.
	_ = keyring.Delete(legacyKeyringService(), legacyAccessTokenKey)
	_ = keyring.Delete(legacyKeyringService(), legacyRefreshTokenKey)
	_ = os.Remove(legacyDesktopAuthFilePath())
}
