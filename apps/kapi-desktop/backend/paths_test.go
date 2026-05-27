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
	if got := namedResourceDir("termbases"); got != filepath.Join("/tmp/iso/kapi", "termbases") {
		t.Fatalf("namedResourceDir(termbases) = %q", got)
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
