package backend

import (
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/host"
	appconfig "github.com/neokapi/neokapi/host/config"
)

// Path resolution for the desktop app's default/system locations. The chains
// themselves live in the shared CLI runtime (host.ConfigDir,
// credentials.DefaultPath, config.GlobalConfigFilePath) so the desktop and
// the kapi CLI derive identical paths; this file only names the desktop's
// views of them.
//
// Every default path is overridable from the environment so the app can run
// fully isolated from the user's real data (tests, demo recordings, CI):
//
//	KAPI_CONFIG_DIR          kapi config root — terms stores, tm, flows,
//	                         format-presets, plugins (shared with the CLI's
//	                         KAPI_HOME convention). Default: <UserConfigDir>/kapi
//	KAPI_DESKTOP_CONFIG_DIR  desktop-only config root — settings.json,
//	                         recent.json. Default: <UserConfigDir>/kapi-desktop
//	KAPI_HOME_DIR            user home — default project location (~/KapiProjects),
//	                         file-dialog defaults. Default: os.UserHomeDir()
//	KAPI_PLUGINS_DIR         plugin discovery root (takes precedence over
//	                         KAPI_CONFIG_DIR/plugins), same variable the CLI
//	                         reads — set KAPI_PLUGINS_DIR_ONLY=1 alongside it to
//	                         skip the user/system roots entirely (dev/CI/sandbox)
//
// On macOS os.UserConfigDir() is ~/Library/Application Support and
// os.UserHomeDir() is $HOME; on Linux they follow XDG.

// kapiConfigDir returns the kapi config root (terms stores, tm, flows, presets,
// plugins) — the shared host.ConfigDir chain.
func kapiConfigDir() string {
	return host.ConfigDir()
}

// desktopConfigDir returns the kapi-desktop config root (settings, recent
// files): the directory of the shared per-app config chain
// (config.GlobalConfigFilePath), which honors KAPI_DESKTOP_CONFIG_DIR.
func desktopConfigDir() string {
	return filepath.Dir(appconfig.GlobalConfigFilePath("kapi-desktop"))
}

// userHomeDir returns the user's home directory, used for default project and
// file-dialog locations.
func userHomeDir() (string, error) {
	if d := os.Getenv("KAPI_HOME_DIR"); d != "" {
		return d, nil
	}
	return os.UserHomeDir()
}
