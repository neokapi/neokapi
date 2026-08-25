package leverage

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
)

// PriorVersionFor returns the last answer approved for a block, and the source
// it was approved for — but only when the governing context has not moved
// since.
//
// The gate is inside on purpose. A prior target is the most anchoring thing
// that can go in a prompt, so offering one approved under superseded rules
// pulls the model back toward wording those rules now reject, and the result is
// stamped with today's fingerprint and looks fresh. A gate a caller applies is
// a gate a caller forgets; there is no way to ask this function for an
// ungoverned answer.
//
// It returns the SOURCE as well as the target because either half alone is
// worse than neither. A target with no source is an anchor with no
// explanation — the model cannot tell which parts still apply. The pair is a
// diff, and the diff is the whole value: it is the reasoning post-editing used
// to force onto a person, handed over with every term it needs.
//
// Not a match, and deliberately unscored. The caller is not choosing between
// candidates; there is one previous answer for a block, and it either still
// stands under the rules in force or it does not.
func PriorVersionFor(
	ctx context.Context,
	vr memory.VersionReader,
	unit, point string,
	sourceLocale, targetLocale model.LocaleID,
	fingerprint string,
) (priorSource, priorTarget string, ok bool) {
	// No chain to walk, and nothing to compare governance against. An empty
	// fingerprint cannot be asserted to match, so an ungoverned run gets no
	// reference rather than an unjudged one.
	if vr == nil || unit == "" || fingerprint == "" {
		return "", "", false
	}

	versions, err := vr.Versions(ctx, memory.VersionQuery{
		Unit:  unit,
		Point: point,
		// One. A chain is history for a surface to show; a prompt wants the
		// answer immediately before this one, and paying tokens for the three
		// before that buys progressively less.
		Limit: 1,
	}, "")
	if err != nil || len(versions) == 0 {
		return "", "", false
	}

	v := versions[0]
	if !v.GovernedBy(fingerprint) {
		return "", "", false
	}

	priorSource = v.Entry.VariantText(sourceLocale)
	priorTarget = v.Entry.VariantText(targetLocale)
	if priorSource == "" || priorTarget == "" {
		return "", "", false
	}
	return priorSource, priorTarget, true
}
