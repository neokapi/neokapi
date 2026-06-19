package selfupdate

import (
	"strings"
	"testing"
)

func TestUpgradeCommand(t *testing.T) {
	cases := map[Source]string{
		SourceHomebrew: "brew upgrade kapi-cli",
		SourceWinget:   "winget upgrade Neokapi.Kapi",
		SourceScoop:    "scoop update kapi",
		SourceTarball:  "", // self-updates
		SourceUnknown:  "",
	}
	for s, want := range cases {
		if got := UpgradeCommand(s, "stable"); got != want {
			t.Errorf("UpgradeCommand(%q, stable) = %q, want %q", s, got, want)
		}
	}
	if got := UpgradeCommand(SourceDeb, "stable"); !strings.Contains(got, "apt") {
		t.Errorf("UpgradeCommand(deb) = %q, want an apt command", got)
	}
	if got := UpgradeCommand(SourceRPM, "stable"); !strings.Contains(got, "dnf") {
		t.Errorf("UpgradeCommand(rpm) = %q, want a dnf command", got)
	}
	// The beta channel must upgrade the @beta Homebrew formula, not stable.
	if got := UpgradeCommand(SourceHomebrew, "beta"); got != "brew upgrade kapi-cli@beta" {
		t.Errorf("UpgradeCommand(homebrew, beta) = %q, want the @beta formula", got)
	}
}

func TestUpgradeArgv(t *testing.T) {
	if got := UpgradeArgv(SourceHomebrew, "stable"); len(got) != 3 || got[2] != "kapi-cli" {
		t.Errorf("UpgradeArgv(homebrew, stable) = %v", got)
	}
	if got := UpgradeArgv(SourceHomebrew, "beta"); len(got) != 3 || got[2] != "kapi-cli@beta" {
		t.Errorf("UpgradeArgv(homebrew, beta) = %v, want kapi-cli@beta", got)
	}
	// apt/dnf must not be auto-run (need sudo / a TTY).
	if got := UpgradeArgv(SourceDeb, "stable"); got != nil {
		t.Errorf("UpgradeArgv(deb) = %v, want nil", got)
	}
	if got := UpgradeArgv(SourceTarball, "stable"); got != nil {
		t.Errorf("UpgradeArgv(tarball) = %v, want nil", got)
	}
}

func TestNotice(t *testing.T) {
	managed := Notice(SourceHomebrew, "stable", "1.0.0", "1.1.0")
	if !strings.Contains(managed, "brew upgrade kapi-cli") {
		t.Errorf("Notice(homebrew) = %q, want the brew command", managed)
	}
	if !strings.Contains(managed, "1.0.0") || !strings.Contains(managed, "1.1.0") {
		t.Errorf("Notice(homebrew) = %q, want both versions", managed)
	}
	beta := Notice(SourceHomebrew, "beta", "1.0.0", "1.2.0-rc.1")
	if !strings.Contains(beta, "brew upgrade kapi-cli@beta") {
		t.Errorf("Notice(homebrew, beta) = %q, want the @beta formula", beta)
	}
	selfUpd := Notice(SourceTarball, "stable", "1.0.0", "1.1.0")
	if !strings.Contains(selfUpd, "kapi update") {
		t.Errorf("Notice(tarball) = %q, want 'kapi update'", selfUpd)
	}
}
