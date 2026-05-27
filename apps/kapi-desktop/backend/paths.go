package backend

import (
	"os"
	"path/filepath"
)

// Path resolution for the desktop app's default/system locations.
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

// kapiConfigDir returns the kapi config root (termbases, tm, flows, presets, plugins).
func kapiConfigDir() string {
	if d := os.Getenv("KAPI_CONFIG_DIR"); d != "" {
		return d
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".config", "kapi")
	}
	return filepath.Join(cfg, "kapi")
}

// desktopConfigDir returns the kapi-desktop config root (settings, recent files).
func desktopConfigDir() string {
	if d := os.Getenv("KAPI_DESKTOP_CONFIG_DIR"); d != "" {
		return d
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".config", "kapi-desktop")
	}
	return filepath.Join(cfg, "kapi-desktop")
}

// userHomeDir returns the user's home directory, used for default project and
// file-dialog locations.
func userHomeDir() (string, error) {
	if d := os.Getenv("KAPI_HOME_DIR"); d != "" {
		return d, nil
	}
	return os.UserHomeDir()
}
