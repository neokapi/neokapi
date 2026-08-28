package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"gopkg.in/yaml.v3"
)

// The arm that has to fetch its own context.
//
// Two arms answer one question: does this guide change the writing? They cannot
// answer the one the product makes — that an assistant plugged into kapi ends up
// with the right context — because the harness put the guide in the prompt
// itself. Nothing in the session shows the agent asking, because it never asked.
//
// So a third arm gets the repository, the same task, the kapi skill, and a
// project with a voice bound to it. No guide in its prompt. It has to notice
// that the project governs its wording and go and get it. Whether it does is
// recorded (AgentRun.KapiCommands) beside what it wrote.
//
// With three arms the two failures are separable: prose no better than bare with
// the guide fetched is context that did not help, and prose no better than bare
// with nothing fetched is a loop that did not close. One arm cannot tell those
// apart, and they have opposite fixes.

// pullProject is the recipe the pulled arm's workspace carries.
//
// A voice bound at the project's default point, which is what `kapi voice guide`
// with no flags resolves and what the skill tells an assistant to run. The
// profile beside it is this point's profile ALREADY RESOLVED (see
// writePulledProject): a real project would resolve the persona from where the
// file sits, and one lab run is one point, so the resolution is done when the
// workspace is built rather than left for the agent to name a flag for.
const pullProject = `version: v1
name: ripgrep-docs
defaults:
  source_language: en
  voice:
    profile_file: .kapi/voice.yaml
collections:
  - name: docs
    content:
      - path: "**/*.md"
`

// prepareWorkspace lays out one run's tree: a pristine copy of the subject, plus
// whatever this arm is given.
//
// Pristine per run, from the tar fetch-lab-repo.sh wrote at clone time, because
// the arms had been sharing one directory and mutating it. An agent ran `cargo
// build` and left 402MB of target/ in the tree every later run then read, so no
// two runs saw the same repository and the difference would have been published
// as a difference between models.
func prepareWorkspace(ctx context.Context, root, home string, arm armSetup) (string, error) {
	tar, err := pristineTar(root)
	if err != nil {
		return "", err
	}
	dest := filepath.Join(home, "workspace")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}
	if out, err := exec.CommandContext(ctx, "tar", "-xf", tar, "-C", dest).CombinedOutput(); err != nil {
		return "", fmt.Errorf("extracting %s: %w: %s", tar, err, out)
	}
	tree := filepath.Join(dest, filepath.Base(LabRepo))
	if _, err := os.Stat(tree); err != nil {
		return "", fmt.Errorf("%s did not contain %s", tar, filepath.Base(LabRepo))
	}
	if !arm.pull {
		return tree, nil
	}
	if err := writePulledProject(tree, arm.profile); err != nil {
		return "", err
	}
	if arm.pointer != "" {
		if err := os.WriteFile(filepath.Join(tree, "CLAUDE.md"), []byte(arm.pointer), 0o644); err != nil {
			return "", err
		}
	}
	return tree, copyTree(filepath.Join(root, "cli", "skills", "data", "kapi"),
		filepath.Join(tree, ".claude", "skills", "kapi"))
}

// armSetup is what one arm's workspace needs beyond the source tree.
type armSetup struct {
	// pull installs the project and the skill, and nothing else does.
	pull bool
	// profile is the voice this point resolves to, written into the project so
	// `kapi voice guide` prints it.
	profile *coreprofile.VoiceProfile
	// pointer is a CLAUDE.md written beside the recipe, telling an assistant
	// that this project's wording is governed and how to retrieve it. Empty in
	// the published arm, which is the point: the sweep measures whether the
	// skill alone is enough. The follow-up experiment (probe_test.go) sets it to
	// find out whether a signpost is what the arm was missing.
	pointer string
}

// labPointer is the three sentences an onboarding step would write.
//
// Deliberately says nothing about HOW to write, only that the project holds a
// voice and where to ask for it. A pointer that carried the guidance would be
// the pushed arm with extra steps.
const labPointer = `# ripgrep

This project's documentation voice is held by kapi, and applies to any prose
written here. Retrieve what is in force before writing, with ` + "`kapi voice guide`" + `.
`

// writePulledProject binds the voice to the workspace the way a project does.
func writePulledProject(tree string, profile *coreprofile.VoiceProfile) error {
	if profile == nil {
		return errors.New("the pulled arm needs a profile to bind")
	}
	body, err := yaml.Marshal(profile)
	if err != nil {
		return fmt.Errorf("marshalling the resolved profile: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(tree, ".kapi"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tree, ".kapi", "voice.yaml"), body, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(tree, "kapi.yaml"), []byte(pullProject), 0o644)
}

// verifyPull runs the command the skill tells an assistant to run, and checks
// that it prints what the pushed arm was handed.
//
// Before the sweep, not after: if the two arms end up with different text the
// comparison is between two governances rather than between two ways of
// delivering one, and the page would say otherwise. A profile that fails to bind
// at all is the same defect wearing a worse disguise — `kapi voice guide` would
// exit non-zero, the agent would shrug and write the bare document, and the arm
// would publish as "the model ignored the context".
func verifyPull(ctx context.Context, kapiBin, tree, want string) error {
	cmd := exec.CommandContext(ctx, kapiBin, "voice", "guide")
	cmd.Dir = tree
	cmd.Env = append(agentEnv(), pullEnv(filepath.Dir(tree), tree)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("`kapi voice guide` in the pulled workspace: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	// The guide, wherever the command frames it. `voice guide` prints a header
	// before the body, so the test is containment rather than equality.
	if got := string(out); !strings.Contains(got, strings.TrimSpace(want)) {
		return fmt.Errorf("the pulled arm would fetch a different guide from the one the pushed arm is given:\n"+
			"`kapi voice guide` printed %d bytes not containing the %d-byte guide.\n"+
			"The two arms would differ in WHAT they were governed by rather than in HOW it arrived",
			len(got), len(want))
	}
	return nil
}

// checkPull builds a throwaway pulled workspace and asks it for the guide.
func checkPull(ctx context.Context, root, kapiBin string, profile *coreprofile.VoiceProfile, want string) error {
	home, err := os.MkdirTemp("", "authoringlab-preflight-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(home)
	tree, err := prepareWorkspace(ctx, root, home, armSetup{pull: true, profile: profile})
	if err != nil {
		return err
	}
	return verifyPull(ctx, kapiBin, tree, want)
}

// pullEnv is the isolation contract for an arm that must find a project.
//
// KAPI_NO_PROJECT would defeat the arm: it opts out of discovery, and discovery
// is the thing being measured. The contract is met the other way, by naming the
// project explicitly — KAPI_PROJECT is consulted before the upward walk, so no
// walk happens and this cannot bind to the repository's own dogfood recipe. The
// throwaway config, plugin and cache roots are unchanged. See CLAUDE.md.
func pullEnv(home, tree string) []string {
	env := []string{"KAPI_PROJECT=" + filepath.Join(tree, "kapi.yaml")}
	for _, kv := range isolationEnv(home) {
		if strings.HasPrefix(kv, "KAPI_NO_PROJECT=") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

// kapiOnlyBin returns a directory holding one command.
//
// The obvious thing is to put bin/ on PATH, and bin/ is every binary this
// repository builds — the server, the plugins, the desktop backend. The arm's
// claim is that an assistant with the kapi CLI can fetch its own context, so the
// kapi CLI is what it gets.
func kapiOnlyBin(home, kapiBin string) (string, error) {
	dir := filepath.Join(home, "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	link := filepath.Join(dir, "kapi")
	if err := os.Symlink(kapiBin, link); err != nil && !os.IsExist(err) {
		return "", err
	}
	return dir, nil
}

// pristineTar names the archive each run is extracted from, and says how to get
// it when it is missing.
func pristineTar(root string) (string, error) {
	tar := filepath.Join(root, LabRepo+".tar")
	if _, err := os.Stat(tar); err != nil {
		return "", fmt.Errorf("%s.tar is not there: run `./scripts/fetch-lab-repo.sh` "+
			"(FORCE_FETCH=1 if the tree predates the archive)", LabRepo)
	}
	return tar, nil
}

// copyTree copies a directory, which here is the shipped skill.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}

// findKapi locates the CLI the pulled arm is given.
func findKapi(root string) (string, error) {
	if p := os.Getenv("KAPI_BIN"); p != "" {
		return p, nil
	}
	p := filepath.Join(root, "bin", "kapi")
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("no kapi at %s: the pulled arm is given the CLI, so build it first "+
			"(`make build`), or set KAPI_BIN", p)
	}
	return p, nil
}
