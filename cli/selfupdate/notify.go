package selfupdate

import "fmt"

// Package-manager identifiers used in the upgrade commands we print. These
// must match how the artifacts are actually published (the Homebrew formula
// name, the winget package Id, etc.).
const (
	brewFormula = "kapi-cli"
	wingetID    = "Neokapi.Kapi"
	scoopApp    = "kapi"
	debPackage  = "kapi"
)

// brewFormulaFor returns the Homebrew formula name for a channel. The fast track
// ships as the @beta-versioned formula, so a beta user must be upgraded against
// it — not the stable formula.
func brewFormulaFor(channel string) string {
	if channel == "beta" {
		return brewFormula + "@beta"
	}
	return brewFormula
}

// UpgradeCommand returns the exact shell command a user should run to upgrade a
// managed install on the given channel, or "" for sources the CLI updates
// itself (run `kapi update`). Only Homebrew currently publishes a beta variant;
// other managers ignore the channel.
func UpgradeCommand(s Source, channel string) string {
	switch s {
	case SourceHomebrew:
		return "brew upgrade " + brewFormulaFor(channel)
	case SourceWinget:
		return "winget upgrade " + wingetID
	case SourceScoop:
		return "scoop update " + scoopApp
	case SourceDeb:
		return "sudo apt update && sudo apt install --only-upgrade " + debPackage
	case SourceRPM:
		return "sudo dnf upgrade " + debPackage
	default:
		return ""
	}
}

// UpgradeArgv returns the argv to execute for an opt-in `kapi update --run` on a
// managed install, or nil for sources we won't auto-run (apt/dnf need sudo/a TTY,
// so we only print those). Returns nil for self-updating sources too.
func UpgradeArgv(s Source, channel string) []string {
	switch s {
	case SourceHomebrew:
		return []string{"brew", "upgrade", brewFormulaFor(channel)}
	case SourceWinget:
		return []string{"winget", "upgrade", wingetID}
	case SourceScoop:
		return []string{"scoop", "update", scoopApp}
	default:
		return nil
	}
}

// Notice is the one-line "update available" message shown after a command runs.
// It is tailored to the install source + channel so the user can act without
// guessing.
func Notice(s Source, channel, current, latest string) string {
	head := fmt.Sprintf("Update available: kapi %s → %s.", current, latest)
	if cmd := UpgradeCommand(s, channel); cmd != "" {
		return head + " Run: " + cmd
	}
	return head + " Run: kapi update"
}
