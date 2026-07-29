package host

import (
	"fmt"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// A target path that cannot hold a localized file is an operational fault, not
// target-language drift: the run is not "behind", it is pointed at a
// destination the filesystem refuses. #1449 is what happens when the two are
// confused — a directory named `fr-FR.json` stat'd clean, the format read
// failed, the failure was classified as an unreadable target, and coverage
// counts an unreadable-but-PRESENT target as fully translated by file-presence.
// The locale therefore read as 100% converged, never became pending, and no
// write was ever attempted, so `kapi up` reported "up to date" over an empty
// directory with no translation anywhere.
//
// The fix has two halves, both here:
//
//  1. blockedTargetPath — measurement. A blocked path holds no translation, so
//     every derivation treats it as missing (untranslated), never as present.
//  2. CheckTargetPathsWritable — admission. A convergence/materialize run
//     pre-flights every destination and refuses to start when any is blocked,
//     naming them all. Nothing has run yet, so no locale loses in-flight work
//     and no AI call is paid for against a destination that cannot be written.
//
// The per-write pre-flight in core/flow stays the backstop for a path that
// becomes blocked mid-run.

// blockedTargetPath reports why a unit's target path cannot hold a localized
// file — a directory occupying it, a device node, an unreadable or
// non-directory parent — or nil when the path is a usable destination (free, or
// holding a regular file to replace).
func blockedTargetPath(path string) error {
	return flow.CheckOutputPath(path)
}

// CheckTargetPathsWritable pre-flights every unit's target path and returns a
// single error naming every blocked destination, or nil when all are usable. It
// is the admission check a run does before doing any work: a destination that
// cannot be written is a fault to fix, and reporting all of them at once lets
// the user clear them in one pass rather than one failed run at a time.
func CheckTargetPathsWritable(units []VerifyUnit) error {
	seen := make(map[string]bool, len(units))
	var errs []error
	for _, u := range units {
		if u.TargetPath == "" || seen[u.TargetPath] {
			continue
		}
		seen[u.TargetPath] = true
		if err := blockedTargetPath(u.TargetPath); err != nil {
			errs = append(errs, err)
		}
	}
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0] // stays the typed *flow.OutputPathError
	default:
		sort.Slice(errs, func(i, j int) bool { return errs[i].Error() < errs[j].Error() })
		return &blockedTargetPathsError{errs: errs}
	}
}

// blockedTargetPathsError aggregates several blocked destinations into one
// report. It keeps each member reachable through errors.As/errors.Is, so a
// caller can still classify the individual faults (*flow.OutputPathError)
// while a person reads them all at once.
type blockedTargetPathsError struct{ errs []error }

func (e *blockedTargetPathsError) Error() string {
	msgs := make([]string, len(e.errs))
	for i, err := range e.errs {
		msgs[i] = err.Error()
	}
	return fmt.Sprintf("%d target paths cannot be written:\n  %s",
		len(e.errs), strings.Join(msgs, "\n  "))
}

func (e *blockedTargetPathsError) Unwrap() []error { return e.errs }

// checkMaterializeTargetsWritable pre-flights the destinations a materialize
// pass would write — every (project source × locale) target — so a blocked path
// is reported before any file is written, rather than leaving some locales
// materialized and the rest not.
func checkMaterializeTargetsWritable(proj *project.KapiProject, root string, files []project.ResolvedFile, locales []model.LocaleID) error {
	units := make([]VerifyUnit, 0, len(files)*len(locales))
	for _, f := range files {
		for _, locale := range locales {
			entry := &project.ExtractionFile{Source: f.Relative}
			target, err := resolveMergeOutputPath(entry, proj, root, locale)
			if err != nil {
				return err
			}
			units = append(units, VerifyUnit{
				SourcePath: f.Path,
				TargetPath: target,
				Locale:     string(locale),
			})
		}
	}
	return CheckTargetPathsWritable(units)
}
