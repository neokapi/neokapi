package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"gopkg.in/yaml.v3"
)

// What the lab writes about, and what governs it at each point.
//
// The subject is ripgrep, cloned at a pinned tag by scripts/fetch-lab-repo.sh.
// A real source tree rather than a brief: a brief hands the model every fact it
// needs in the order it needs them, which measures expansion and not
// documentation. A person writing docs for a tool reads the tool.
//
// The profile is ripgrep's own voice, inferred by kapi from README.md, GUIDE.md
// and FAQ.md and then corrected by hand. What the correction changed is in the
// git history of the file beside this one, and is itself a measurement of how
// far inference gets.

//go:embed corpus/ripgrep-inferred.yaml
var ripgrepProfileYAML []byte

// LabRepo is the pinned tree the agent reads, relative to the repo root.
const LabRepo = "lab-repo/ripgrep-14.1.1"

// Point is one coordinate in the context space, and what is bound there.
type Point struct {
	// Audience is the declared axis value. The coordinate map is open — only
	// product and channel are structural — so a project names the dimensions
	// its content actually varies along, and this one varies by reader.
	Audience string
	// Label is how the page names it.
	Label string
	// Persona is the key in the profile's own `personas:` map. The lab does not
	// hand-build two profiles: it resolves ONE profile at two personas through
	// core/profile.ResolveProfile, which is the mechanism a project would use.
	// Persona vocabulary can only tighten, so neither point can re-allow what
	// ripgrep's voice forbids.
	Persona string
	// Task is the deliverable, stated in BOTH arms.
	//
	// It names the reader and what to produce, and says NOTHING about how to
	// write it. The bare arm has to know who it is writing for, or the
	// comparison measures whether the model was told the audience rather than
	// what the governance did. But the first version went further — "not a
	// programmer: they have a terminal open and no interest in how the tool is
	// built" — which instructs the bare arm to avoid implementation detail, the
	// exact thing the coordinate exists to do. Both arms were steered and only
	// one was credited, so the effect measured itself smaller than it is.
	//
	// The test for whether a line belongs here: does it describe the
	// DELIVERABLE or the READER? Anything describing the PROSE belongs in the
	// profile.
	Task string
}

var points = []Point{
	{
		Audience: "end-user",
		Label:    "End user (non-technical)",
		Persona:  "end-user",
		Task: "Read this repository and write a user guide, in Markdown, of roughly 800 words, " +
			"for ripgrep's file-type filtering: what `--type`, `--type-not`, `--type-add` and " +
			"`--type-list` do and how someone uses them. The reader searches text for a living. " +
			"Ground every claim in what the source and the existing docs actually say.",
	},
	{
		Audience: "developer",
		Label:    "Developer",
		Persona:  "developer",
		Task: "Read this repository and write developer documentation, in Markdown, of roughly " +
			"800 words, for ripgrep's file-type filtering: how type definitions are represented, " +
			"how a matcher is built from them, and what a contributor changing this area needs to " +
			"know. The reader is a Rust programmer reading the crates. Ground every claim in what " +
			"the source actually does.",
	},
}

// labModels is the matrix's model axis.
//
// Ids rather than aliases, and each run records the model that answered rather
// than the one that was asked for, so a silent alias resolution cannot be read
// as a comparison between two models that were in fact one.
var labModels = []string{
	"claude-opus-5",
	"claude-sonnet-5",
	"claude-opus-4-8",
	"claude-haiku-4-5",
}

// loadProfile reads ripgrep's voice, embedded so a run needs nothing but the
// binary and the cloned tree.
func loadProfile() (*coreprofile.VoiceProfile, error) {
	var p coreprofile.VoiceProfile
	if err := yaml.Unmarshal(ripgrepProfileYAML, &p); err != nil {
		return nil, fmt.Errorf("ripgrep profile: %w", err)
	}
	if probs := coreprofile.ValidateProfile(&p); len(probs) > 0 {
		return nil, fmt.Errorf("ripgrep profile is not usable: %v", probs)
	}
	return &p, nil
}

// guideFor renders what the governed arm receives at one point: the profile
// resolved at that persona, through the FULL renderer.
//
// The full one, deliberately. RenderVoiceGuideCompact drops the before/after
// examples entirely, and those are the strongest steering the profile carries
// (#2241). The compact form is what the translation path uses; an assistant
// handed `kapi voice guide` gets this.
func guideFor(base *coreprofile.VoiceProfile, p Point) (string, error) {
	resolved := coreprofile.ResolveProfile(base, "", "", p.Persona)
	if resolved == nil {
		return "", fmt.Errorf("%s: profile resolved to nothing", p.Audience)
	}
	guide := coreprofile.RenderVoiceGuide(resolved)
	if guide == "" {
		return "", fmt.Errorf("%s: the guide rendered empty, so the governed arm "+
			"would be identical to the bare one", p.Audience)
	}
	return guide, nil
}

// repoDir is the cloned tree, and says how to get it when it is missing.
func repoDir(root string) (string, error) {
	dir := filepath.Join(root, LabRepo)
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("%s is not there: run `./scripts/fetch-lab-repo.sh` first", LabRepo)
	}
	return dir, nil
}
