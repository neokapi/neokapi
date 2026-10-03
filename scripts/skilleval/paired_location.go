package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
)

// pairedDiscoverable are the names an agent host or kapi finds by walking up
// from its working directory: a checkout, an instruction file, a host's
// project configuration or skills, a kapi recipe or state directory.
var pairedDiscoverable = []string{
	".git", "CLAUDE.md", "CLAUDE.local.md", "AGENTS.md", "AGENTS.override.md",
	".claude", ".agents", ".codex", "kapi.yaml", ".kapi",
}

// pairedHomeConfig are the developer's own host configuration directories in
// their home directory. Each cell replaces them through HOME, CLAUDE_CONFIG_DIR
// and CODEX_HOME, and the surface probe proves nothing of them is visible.
var pairedHomeConfig = []string{".claude", ".agents", ".codex"}

// checkPairedLocation refuses a study directory with anything discoverable
// above it. Cells live inside the study directory, so a study kept in a
// checkout (harness/out, say) would let Claude read the checkout's CLAUDE.md,
// Codex its AGENTS.md and skills, and an unbound kapi its recipe: none of which
// the surface probe can see, because a host lists no instruction file it
// loaded.
func checkPairedLocation(dir string) error {
	// Claude Code keeps its temporary files in /tmp/claude-<uid>, and a study
	// run from a Claude Code session defaults there. In a cell under it the
	// cell's own Claude Code could not start its shell: sandbox-exec refused
	// to execute it.
	if claudeTemp := filepath.Join("/tmp", "claude-"+strconv.Itoa(os.Getuid())); pairedWithin(dir, claudeTemp) {
		return fmt.Errorf("the study directory %s lies under %s, Claude Code's temporary directory, "+
			"where the Claude Code in a cell cannot start its shell; choose another directory", dir, claudeTemp)
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		home = pairedResolve(home)
	}
	for current := pairedResolve(dir); ; current = filepath.Dir(current) {
		for _, name := range pairedDiscoverable {
			if current == home && slices.Contains(pairedHomeConfig, name) {
				continue
			}
			if _, err := os.Lstat(filepath.Join(current, name)); err == nil {
				return fmt.Errorf("the study directory %s lies under %s, which holds %s that an agent in a cell would find; "+
					"choose a directory outside any checkout, such as one under the system temporary directory", dir, current, name)
			}
		}
		if parent := filepath.Dir(current); parent == current {
			return nil
		}
	}
}
