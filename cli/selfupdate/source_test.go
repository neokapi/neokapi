package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect_EnvOverrideWins(t *testing.T) {
	t.Setenv("KAPI_INSTALL_SOURCE", "Homebrew")
	if got := Detect(); got != SourceHomebrew {
		t.Fatalf("Detect() = %q, want %q (env override, case-insensitive)", got, SourceHomebrew)
	}
}

func TestSourceFromPath(t *testing.T) {
	cases := []struct {
		path string
		want Source
	}{
		{"/opt/homebrew/Cellar/kapi-cli/1.1.0/bin/kapi", SourceHomebrew},
		{"/home/linuxbrew/.linuxbrew/bin/kapi", SourceHomebrew},
		{`C:\Users\me\AppData\Local\Microsoft\WinGet\Packages\Neokapi.Kapi\kapi.exe`, SourceWinget},
		{`C:\Users\me\scoop\apps\kapi\current\kapi.exe`, SourceScoop},
		{"/usr/bin/kapi", SourceUnknown}, // deb/rpm left to the build stamp
		{"/home/me/.local/bin/kapi", SourceUnknown},
	}
	for _, tc := range cases {
		if got := sourceFromPath(tc.path); got != tc.want {
			t.Errorf("sourceFromPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestManaged(t *testing.T) {
	managed := []Source{SourceHomebrew, SourceWinget, SourceScoop, SourceDeb, SourceRPM}
	for _, s := range managed {
		if !s.Managed() {
			t.Errorf("%q.Managed() = false, want true", s)
		}
	}
	for _, s := range []Source{SourceTarball, SourceUnknown} {
		if s.Managed() {
			t.Errorf("%q.Managed() = true, want false", s)
		}
	}
}

func TestCanSelfReplace(t *testing.T) {
	// Managed installs never self-replace, regardless of writability.
	if CanSelfReplace(SourceHomebrew) {
		t.Error("CanSelfReplace(homebrew) = true, want false")
	}
}

func TestDirWritable(t *testing.T) {
	dir := t.TempDir()
	if !dirWritable(dir) {
		t.Errorf("dirWritable(%q) = false, want true", dir)
	}
	missing := filepath.Join(dir, "does-not-exist")
	if dirWritable(missing) {
		t.Errorf("dirWritable(%q) = true, want false (missing dir)", missing)
	}
}

func TestBinaryName(t *testing.T) {
	// Sanity: a non-empty name that ends in kapi[.exe].
	n := binaryName()
	if n != "kapi" && n != "kapi.exe" {
		t.Fatalf("binaryName() = %q", n)
	}
}

func TestExecutablePath(t *testing.T) {
	p, err := ExecutablePath()
	if err != nil {
		t.Fatalf("ExecutablePath() error: %v", err)
	}
	if !filepath.IsAbs(p) {
		t.Errorf("ExecutablePath() = %q, want absolute", p)
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("ExecutablePath() %q does not exist: %v", p, err)
	}
}
