package host

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// qaCategoryTargetSameAsSource is the check finding raised for a target
// identical to its source.
const qaCategoryTargetSameAsSource = "target-same-as-source"

// identicalTargetRule is the one definition of when a target identical to its
// source is a settled outcome rather than a unit nobody translated. Both
// surfaces that feed the ship gate ask it, and they must ask it the same way:
// `kapi check`'s checks gate reports the finding and fails on it, and
// computeLoopCheckExclusions demotes the unit below `translated` for the same
// gate during `kapi up`. With the suppression on one surface and not the other,
// the same project, the same string and the same gate got two answers — check
// passed the unit and the convergence coverage held it back.
//
// Two things settle the question, and a unit needs only one of them:
//
//   - The project's terms keep the string unchanged: an entry whose target is
//     its source (a product name, a proper noun). Identity is what a correct
//     target looks like for it.
//   - The project's committed record APPROVES the unit in this locale, with both
//     halves of the pairing it blessed still on disk. The rule is a heuristic for
//     "nobody translated this", and an approval is a person having read the exact
//     pairing and answered that question. A rejection is a decision too and
//     settles nothing; neither does an approval whose translation has been edited
//     since, or one whose source has moved — approvesTarget is the same reading
//     the record absorber takes of the same decision.
//
// It is narrow on purpose: only target-same-as-source is suppressed. A dropped
// placeholder on an approved unit is still a defect, and an approval does not
// license it.
type identicalTargetRule struct {
	// resolve reads the do-not-translate terms for a locale; nil disables that
	// half (no terms bound, or none resolvable).
	resolve  func(locale string) map[string]bool
	byLocale map[string]map[string]bool
	reviewed reviewedIndex
	// root is the project root the rule resolves a unit's source path against,
	// so it asks the index for the decision made in THAT document — the same
	// identity the decision was recorded under.
	root string
	// docs turns that path into the document's durable key, so a file that was
	// renamed since the decision still answers for it.
	docs DocumentIndex
}

// newIdenticalTargetRule builds the rule from the project's terms and its
// committed decisions.
//
// The decisions are read only from a store that already exists. A gate is a read:
// opening the project store CREATES it, migrations and all, so a `kapi check` on
// a project that has never run would leave one behind — and a project with no
// store has recorded no decision, which is exactly the empty index it yields.
func (a *App) newIdenticalTargetRule(ctx context.Context, cmd Command, proj *project.KapiProject, root string) (*identicalTargetRule, error) {
	r := &identicalTargetRule{
		byLocale: map[string]map[string]bool{},
		resolve:  func(locale string) map[string]bool { return a.doNotTranslateTerms(cmd, locale) },
		root:     root,
		docs:     a.documentIndexOrEmpty(ctx, root),
	}
	if !projectStoreExists(root) {
		return r, nil
	}
	// Past the stat the store is there, so a failure to read it is a fault, not
	// an absence. Swallowing it would silently drop every approval and report
	// approved wording as untranslated — the defect this rule exists to close,
	// re-introduced as a degradation.
	idx, err := a.loadReviewedCorrections(ctx, proj, root)
	if err != nil {
		return nil, fmt.Errorf("read the project's decisions for the checks gate: %w", err)
	}
	r.reviewed = idx
	return r, nil
}

// settles reports whether the project's own record answers the identity for this
// block, in this locale, in the document sourcePath names.
func (r *identicalTargetRule) settles(sourcePath string, b *model.Block, locale string) bool {
	if r == nil {
		return false
	}
	if r.doNotTranslate(locale)[b.SourceText()] {
		return true
	}
	e, _, applies := r.reviewed.grade(r.docs.Scope(r.root, sourcePath), b, locale)
	return approvesTarget(e, applies)
}

// suppresses reports whether a check finding is answered by the project's record,
// and so is not a defect to report or to gate on.
func (r *identicalTargetRule) suppresses(f check.Finding, sourcePath string, b *model.Block, locale string) bool {
	return f.Category == qaCategoryTargetSameAsSource && r.settles(sourcePath, b, locale)
}

// doNotTranslate returns the locale's do-not-translate source texts, resolving
// them once per locale (the resolution opens the terms store).
func (r *identicalTargetRule) doNotTranslate(locale string) map[string]bool {
	if set, ok := r.byLocale[locale]; ok {
		return set
	}
	var set map[string]bool
	if r.resolve != nil {
		set = r.resolve(locale)
	}
	r.byLocale[locale] = set
	return set
}
