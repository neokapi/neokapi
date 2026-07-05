package backend

import (
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/cli"
	appconfig "github.com/neokapi/neokapi/cli/config"
)

// Path resolution for the desktop app's default/system locations. The chains
// themselves live in the shared CLI runtime (cli.ConfigDir,
// credentials.DefaultPath, config.GlobalConfigFilePath) so the desktop and
// the kapi CLI derive identical paths; this file only names the desktop's
// views of them.
//
// Every default path is overridable from the environment so the app can run
// fully isolated from the user's real data (tests, demo recordings, CI):
//
//	KAPI_CONFIG_DIR          kapi config root — termbases, tm, flows,
//	                         format-presets, plugins (shared with the CLI's
//	                         KAPI_HOME convention). Default: <UserConfigDir>/kapi
//	KAPI_DESKTOP_CONFIG_DIR  desktop-only config root — settings.json,
//	                         recent.json. Default: <UserConfigDir>/kapi-desktop
//	KAPI_HOME_DIR            user home — default project location (~/KapiProjects),
//	                         file-dialog defaults. Default: os.UserHomeDir()
//	KAPI_PLUGIN_DIR          plugin dir (takes precedence over KAPI_CONFIG_DIR/plugins)
//
// On macOS os.UserConfigDir() is ~/Library/Application Support and
// os.UserHomeDir() is $HOME; on Linux they follow XDG.

// kapiConfigDir returns the kapi config root (termbases, tm, flows, presets,
// plugins) — the shared cli.ConfigDir chain.
func kapiConfigDir() string {
	return cli.ConfigDir()
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
