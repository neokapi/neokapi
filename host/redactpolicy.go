package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// A declared redaction policy is a property of the content, not of the command
// that moves it. Every route that sends content out of kapi (a push, a run of
// one tool, a flow, the deeper look, the record of an edit) asks for the
// policy of the file the content sits in, through RedactionFor, and takes no
// flags: flags belong to a command, and the policy belongs to the project.
//
// A recipe declares a policy under `defaults.redaction`, on a collection and
// on a content item (project.RedactionForPath). The most specific declaration
// governs a file and replaces the others whole.

// RedactionFor returns the redaction policy that governs the file at relPath
// in project p, or nil when none does. relPath is project-relative and
// slash-separated; "" names content that sits in no file, which
// `defaults.redaction` governs.
//
// Recipe only. `kapi extract` layers --redact / --redact-rules on top of this
// for ad-hoc runs (see resolveRedaction); every other route applies the
// declared policy and nothing else.
func RedactionFor(p *project.KapiProject, relPath string) *project.RedactionSpec {
	return p.RedactionForPath(relPath).Spec
}

// RedactionDeclared reports whether p declares a redaction policy for any of
// its files. A route that cannot name the file it guards (a flow whose tools
// serve every file of a run, the vault a route prepares before it reads)
// asks this.
func RedactionDeclared(p *project.KapiProject) bool {
	return p.DeclaresRedaction()
}

// redactionPath is the path RedactionFor reads for a file a route names: its
// path relative to the project root, or "" for no file and for a file outside
// the root, which `defaults.redaction` governs. Relative paths are read
// against the working directory.
func redactionPath(root, path string) string {
	if root == "" || path == "" {
		return ""
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	rel, _ := projectRelPath(absRoot, path)
	return rel
}

// RedactAtIngest applies a declared policy to blocks as they enter kapi, before
// anything hashes, stores or transmits them, vaulting the originals at
// vaultPath.
//
// Ingest is the only placement that makes the guarantee cheap. Redact mid-flow
// and every step before it has held the original, so safety depends on ordering
// and the ordering has to be checked (that is what ValidatePlacement does).
// Redact at ingest and the block store never holds an original, so every
// downstream destination (server, model, connector, reviewer) is safe by
// construction.
//
// It is a no-op when spec is nil, so callers can invoke it unconditionally.
func RedactAtIngest(
	ctx context.Context,
	blocks []*model.Block,
	spec *project.RedactionSpec,
	rootDir, vaultPath string,
	sourceLocale model.LocaleID,
) error {
	if spec == nil || len(blocks) == 0 {
		return nil
	}
	if err := checkRedactionRules(spec); err != nil {
		return err
	}
	if vaultPath == "" {
		return errors.New("redaction is enabled but no vault path was given: refusing to redact without somewhere to keep the originals")
	}
	if err := ensureVaultDir(vaultPath); err != nil {
		return err
	}
	return applyRedaction(ctx, blocks, spec, rootDir, vaultPath, sourceLocale)
}

// checkRedactionRules refuses a policy that would redact nothing: rules
// detection with no rules file finds no span, silently. Saying so keeps the
// project's owner from believing the content is protected.
func checkRedactionRules(spec *project.RedactionSpec) error {
	if spec.Rules == "" && !hasDetector(spec.Detectors, "entities") {
		return errors.New("redaction is enabled but no rules file is set. Add rules: to the redaction policy in the recipe (defaults.redaction, or the redaction: of the collection or content item that holds the file)")
	}
	return nil
}

func hasDetector(detectors []string, want string) bool {
	return slices.Contains(detectors, want)
}

func ensureVaultDir(vaultPath string) error {
	return os.MkdirAll(filepath.Dir(vaultPath), 0o700)
}
