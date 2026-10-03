package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
)

// pairedFilesAlias is the multi-call name the project-free condition runs the
// binary under (cli.FilesAliasName). The evaluation links it in each cell;
// nothing else installs it.
const pairedFilesAlias = "kapi-files"

// pairedArm is the kapi surface one condition installs in a cell. Every cell
// keeps the ordinary shell and file tools; an arm adds at most one skill, the
// MCP server, and the kapi names on the cell's PATH.
type pairedArm struct {
	// Skill is the name of the skill installed in the workspace, "" for none.
	Skill string
	// MCP reports whether the kapi MCP server is configured for the host.
	MCP bool
	// Executables are the kapi names on the cell's PATH.
	Executables []string
	// Project reports whether the kapi the cell runs is bound to the fixture's
	// recipe. The project-free arm runs with discovery off.
	Project bool
}

// pairedConditions lists the conditions a study may name, in the order the
// reports print them.
var pairedConditions = []string{"baseline", "skill-cli", "mcp", "project-free"}

func pairedArmFor(condition string) (pairedArm, error) {
	switch condition {
	case "baseline":
		return pairedArm{Executables: []string{}}, nil
	case "skill-cli":
		return pairedArm{Skill: "kapi", Executables: []string{"kapi"}, Project: true}, nil
	case "mcp":
		return pairedArm{MCP: true, Executables: []string{"kapi"}, Project: true}, nil
	case "project-free":
		return pairedArm{Skill: pairedFilesAlias, Executables: []string{pairedFilesAlias}}, nil
	}
	return pairedArm{}, fmt.Errorf("unknown condition %q", condition)
}

// pairedSkillDir returns where a host discovers a skill installed in the
// workspace.
func pairedSkillDir(workspace, host, skill string) string {
	if host == "codex" {
		return filepath.Join(workspace, ".agents", "skills", skill)
	}
	return filepath.Join(workspace, ".claude", "skills", skill)
}

// installPairedSkill copies the arm's skill into the workspace. The kapi skill
// is the one the binary ships (cli/skills/data/kapi), from the study's own copy
// in a live phase; the project-free skill is the evaluation's own text for the
// alias, embedded with the corpus.
func installPairedSkill(launch PairedLaunch, arm pairedArm) error {
	if arm.Skill == "" {
		return nil
	}
	destination := pairedSkillDir(launch.Workspace, launch.Agent.Host, arm.Skill)
	if arm.Skill == "kapi" {
		source := launch.SkillSource
		if source == "" {
			source = filepath.Join(launch.RepoRoot, "cli", "skills", "data", "kapi")
		}
		return copyTree(source, destination)
	}
	prefix := "testdata/paired/skills/" + arm.Skill
	return fs.WalkDir(pairedFixtures, prefix, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		target := filepath.Join(destination, filepath.FromSlash(name[len(prefix):]))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		data, err := pairedFixtures.ReadFile(name)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
}

// agentKapi is the kapi binary the agent's own commands run.
func (l PairedLaunch) agentKapi() string {
	if l.CellKapi != "" {
		return l.CellKapi
	}
	return l.KapiBin
}

// linkPairedKapi puts the binary under test into the cell, as a hard link
// where the checkout and the cell share a volume and as a copy otherwise. The
// agent's sandbox then needs no read access into the checkout, which holds
// the evaluator's references, and its shell never execs a file under a path
// the sandbox denies.
func linkPairedKapi(launch PairedLaunch) (string, error) {
	dir := filepath.Join(launch.StateDir, "kapi")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	destination := filepath.Join(dir, "kapi")
	if _, err := os.Lstat(destination); err == nil {
		return destination, nil
	}
	if err := os.Link(launch.KapiBin, destination); err == nil {
		return destination, nil
	}
	source, err := os.Open(launch.KapiBin)
	if err != nil {
		return "", err
	}
	defer source.Close()
	target, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(target, source); err != nil {
		_ = target.Close()
		return "", err
	}
	return destination, target.Close()
}

// pairedKapiNames are the names a kapi binary answers to in a cell.
var pairedKapiNames = []string{"kapi", "kcat", "kgrep", "ksed", "kdiff", "kconv", pairedFilesAlias}

// pairedToolboxNames run as subcommands of kapi and of the alias.
var pairedToolboxNames = []string{"kcat", "kgrep", "ksed", "kdiff", "kconv"}

func pairedKapiExecutable(word string) bool {
	return slices.Contains(pairedKapiNames, path.Base(filepath.ToSlash(word)))
}
