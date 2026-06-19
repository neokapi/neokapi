package backend

import (
	"context"
	"crypto/ed25519"
	_ "embed"
	"encoding/base64"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/version"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/appcast"
)

// updatePublicKeyB64 is the base64 ed25519 public key that signs the appcast
// feed (the trust anchor for in-app updates). It is committed; the matching
// private key lives only in CI (UPDATE_ED25519_PRIVATE_KEY). Generate the pair
// with `go run ./scripts/mkappcast keygen`. Until a real key is committed the
// placeholder fails to decode and the updater refuses signed releases (fail
// closed) — see updatePublicKey.
//
//go:embed update-ed25519.pub
var updatePublicKeyB64 []byte

const (
	// appcastBaseURL hosts the signed appcast feeds (GitHub Pages of the
	// registry repo, alongside cli.json).
	appcastBaseURL = "https://neokapi.github.io/registry"
	// appcastName is this app's feed basename.
	appcastName = "bowrain"
)

// updateChannel mirrors the kapi CLI: stable by default, KAPI_UPDATE_CHANNEL
// (e.g. "beta") to follow the fast track.
func updateChannel() string {
	if c := strings.TrimSpace(os.Getenv("KAPI_UPDATE_CHANNEL")); c != "" {
		return c
	}
	return "stable"
}

// feedURL returns the appcast URL for a channel. Each channel gets its own feed
// so a stable build is never offered a beta item.
func feedURL(channel string) string {
	if channel == "beta" {
		return appcastBaseURL + "/appcast-" + appcastName + "-beta.xml"
	}
	return appcastBaseURL + "/appcast-" + appcastName + ".xml"
}

// updatePublicKey decodes the embedded key, returning nil (fail closed) when it
// is the placeholder or otherwise not a 32-byte ed25519 key.
func updatePublicKey() []byte {
	k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(updatePublicKeyB64)))
	if err != nil || len(k) != ed25519.PublicKeySize {
		return nil
	}
	return k
}

// InitUpdater wires the Wails native updater against the appcast feed for the
// current channel, with a periodic background check. Best-effort: a
// misconfigured updater must never block app startup.
func InitUpdater(app *application.App) {
	ch := updateChannel()
	ac, err := appcast.New(appcast.Config{URL: feedURL(ch), Channel: ch})
	if err != nil {
		slog.Warn("updater: appcast provider", "error", err)
		return
	}
	if err := app.Updater.Init(updater.Config{
		CurrentVersion: version.Version,
		Providers:      []updater.Provider{ac},
		PublicKey:      updatePublicKey(),
		Channel:        ch,
		CheckInterval:  6 * time.Hour,
	}); err != nil {
		slog.Warn("updater: init", "error", err)
	}
}

// CheckForUpdatesNow runs the full check → download → verify → swap → relaunch
// flow. Runs off the UI thread. Exposed for a future "Check for Updates…" menu
// item / frontend action.
func CheckForUpdatesNow(app *application.App) {
	go func() {
		if err := app.Updater.CheckAndInstall(context.Background()); err != nil {
			slog.Error("updater: check and install", "error", err)
		}
	}()
}
