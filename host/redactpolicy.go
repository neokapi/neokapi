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

// A declared redaction policy is a property of the CONTENT, not of the command
// that happens to be moving it.
//
// It did not start that way. `defaults.redaction` had exactly one consumer —
// resolveRedaction, called from `kapi extract` — so a project that declared its
// content sensitive was heard by one verb and ignored by every other. `kapi
// push` read the policy nowhere at all, which meant originals reached the
// server whenever the operator used the wrong verb.
//
// That failure mode is worse than the flags retired alongside it. Forgetting
// --no-brand carried governance; forgetting --concepts did an ordinary push.
// Both failed safe. A policy that is simply not consulted fails OPEN.
//
// So the resolution lives here, callable from any ingest point, and takes no
// flags: flags belong to a command, and this is a property of the project.

// ProjectRedaction returns the redaction policy a project declares, or nil when
// it declares none.
//
// Recipe only — deliberately. `kapi extract` layers --redact / --redact-rules
// on top of this for ad-hoc, project-less runs (see resolveRedaction), and that
// is the correct place for a flag. Every non-interactive ingest path wants the
// declared policy and nothing else.
func ProjectRedaction(p *project.KapiProject) *project.RedactionSpec {
	if p == nil || p.Defaults.Redaction == nil || !p.Defaults.Redaction.Enabled {
		return nil
	}
	spec := *p.Defaults.Redaction
	return &spec
}

// RedactAtIngest applies a declared policy to blocks as they enter kapi, before
// anything hashes, stores or transmits them, vaulting the originals at
// vaultPath.
//
// Ingest is the only placement that makes the guarantee cheap. Redact mid-flow
// and every step before it has held the original, so safety depends on ordering
// and the ordering has to be checked (that is what ValidatePlacement does).
// Redact at ingest and the block store never holds an original, so every
// downstream destination — server, model, connector, reviewer — is safe by
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
	if spec.Rules == "" && !hasDetector(spec.Detectors, "entities") {
		// Rules detection with no rules file redacts nothing, silently. Say so
		// rather than let a project believe it is protected.
		return errors.New("redaction is enabled but no rules file is set. Add defaults.redaction.rules to the recipe")
	}
	if vaultPath == "" {
		return errors.New("redaction is enabled but no vault path was given: refusing to redact without somewhere to keep the originals")
	}
	if err := ensureVaultDir(vaultPath); err != nil {
		return err
	}
	return applyRedaction(ctx, blocks, spec, rootDir, vaultPath, sourceLocale)
}

func hasDetector(detectors []string, want string) bool {
	return slices.Contains(detectors, want)
}

func ensureVaultDir(vaultPath string) error {
	return os.MkdirAll(filepath.Dir(vaultPath), 0o700)
}
