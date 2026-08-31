package backend

import (
	"path/filepath"
	"testing"
)

func TestKapiConfigDirHonorsEnv(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", "/tmp/iso/kapi")
	if got := kapiConfigDir(); got != "/tmp/iso/kapi" {
		t.Fatalf("kapiConfigDir = %q, want /tmp/iso/kapi", got)
	}
	// namedResourceDir composes onto the overridden root.
	if got := namedResourceDir("terms"); got != filepath.Join("/tmp/iso/kapi", "terms") {
		t.Fatalf("namedResourceDir(terms stores) = %q", got)
	}
}

func TestKapiConfigDirDefault(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", "")
	got := kapiConfigDir()
	if filepath.Base(got) != "kapi" {
		t.Fatalf("default kapiConfigDir = %q, want it to end in /kapi", got)
	}
}

func TestDesktopConfigDirHonorsEnv(t *testing.T) {
	t.Setenv("KAPI_DESKTOP_CONFIG_DIR", "/tmp/iso/desktop")
	if got := desktopConfigDir(); got != "/tmp/iso/desktop" {
		t.Fatalf("desktopConfigDir = %q, want /tmp/iso/desktop", got)
	}
}

func TestUserHomeDirHonorsEnv(t *testing.T) {
	t.Setenv("KAPI_HOME_DIR", "/tmp/iso/home")
	got, err := userHomeDir()
	if err != nil || got != "/tmp/iso/home" {
		t.Fatalf("userHomeDir = %q, err=%v, want /tmp/iso/home", got, err)
	}
}

// defaultPluginDir must read the same KAPI_PLUGINS_DIR the CLI and the
// desktop's own venue dispatch honour, or a developer isolating one surface
// (e.g. via KAPI_PLUGINS_DIR_ONLY) silently doesn't isolate the other —
// exactly the "plugin declared in both ..." duplicate the mismatch produced.
func TestDefaultPluginDirHonorsEnv(t *testing.T) {
	t.Setenv("KAPI_PLUGINS_DIR", "/tmp/iso/plugins")
	if got := defaultPluginDir(); got != "/tmp/iso/plugins" {
		t.Fatalf("defaultPluginDir = %q, want /tmp/iso/plugins", got)
	}
}

func TestDefaultPluginDirFallsBackToConfigDir(t *testing.T) {
	t.Setenv("KAPI_PLUGINS_DIR", "")
	t.Setenv("KAPI_CONFIG_DIR", "/tmp/iso/kapi")
	want := filepath.Join("/tmp/iso/kapi", "plugins")
	if got := defaultPluginDir(); got != want {
		t.Fatalf("defaultPluginDir = %q, want %q", got, want)
	}
}
