package server

import (
	"context"
	"fmt"
	"slices"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	coretools "github.com/neokapi/neokapi/core/tools"
)

// streamCommitCheck is the server's change.CommitCheck: each changed edition
// held to the checks in force where its item sits (checksAtPoint), resolved
// the way the editor's check routes and the ship gate resolve them.
//
//   - A translation meets the standard per-locale checks (the rule check with
//     placeholder integrity against the source beside it, and the protected
//     terms) and the term gate: a forbidden or competitor term it uses, or a
//     rendering a source concept mandates and it omits, is a failing
//     terms.vocabulary finding.
//   - The source meets the vocabulary governing the point: the voice
//     profile's rules and the workspace's retired, forbidden and competitor
//     terms.
//
// Every analyzer is deterministic; a model-backed check never runs at commit.
// The governance is resolved once per item and language within one call.
type streamCommitCheck struct {
	s      *Server
	proj   *store.Project
	stream string
	wsID   string
	wsSlug string

	points   map[pointKey]pointChecks
	gate     *termGate
	gateRead bool
}

type pointKey struct {
	item   string
	locale model.LocaleID
}

var _ change.CommitCheck = (*streamCommitCheck)(nil)

func (k *streamCommitCheck) point(ctx context.Context, item string, locale model.LocaleID) (pointChecks, error) {
	key := pointKey{item, locale}
	if pc, ok := k.points[key]; ok {
		return pc, nil
	}
	pc, err := k.s.checksAtPoint(ctx, k.proj.ID, k.stream, item, k.wsID, k.wsSlug, locale)
	if err != nil {
		return pc, err
	}
	if k.points == nil {
		k.points = map[pointKey]pointChecks{}
	}
	k.points[key] = pc
	return pc, nil
}

func (k *streamCommitCheck) termGate(ctx context.Context) *termGate {
	if !k.gateRead {
		k.gate = k.s.resolveTermGate(ctx, k.proj, k.stream, k.wsID)
		k.gateRead = true
	}
	return k.gate
}

// Check returns the findings on each changed edition before and after the
// change, and the fingerprint of the governance the term gate applied.
func (k *streamCommitCheck) Check(ctx context.Context, changes []change.EditionChange) ([]change.CheckOutcome, string, error) {
	source := k.proj.DefaultSourceLanguage
	type blockRef struct{ doc, block string }
	sources := map[blockRef]int{}
	for i, ch := range changes {
		if ch.Role == change.RoleAuthoritative {
			sources[blockRef{ch.Ref.Doc, ch.Ref.Block}] = i
		}
	}
	outcomes := make([]change.CheckOutcome, len(changes))
	locales := []string{}
	for i, ch := range changes {
		locale := source
		if ch.Role == change.RoleDerived {
			locale = ch.Ref.Edition.Locale
		}
		if !slices.Contains(locales, string(locale)) {
			locales = append(locales, string(locale))
		}
		pc, err := k.point(ctx, ch.Ref.Doc, locale)
		if err != nil {
			return nil, "", fmt.Errorf("resolve the checks for %s: %w", ch.Ref.Doc, err)
		}
		for _, before := range []bool{true, false} {
			runs := ch.After
			if before {
				if ch.Before == nil {
					continue // the change set created the edition
				}
				runs = ch.Before
			} else if ch.AfterRev == model.AbsentRevision {
				continue // the change set removed the edition
			}
			var found []change.Finding
			if ch.Role == change.RoleAuthoritative {
				found, err = k.sourceFindings(ctx, ch, runs, pc)
			} else {
				srcRuns := sourceRuns(ch.Block)
				if j, ok := sources[blockRef{ch.Ref.Doc, ch.Ref.Block}]; ok && before {
					srcRuns = changes[j].Before
				}
				found, err = k.targetFindings(ctx, ch, srcRuns, runs, pc)
			}
			if err != nil {
				return nil, "", err
			}
			if before {
				outcomes[i].Before = found
			} else {
				outcomes[i].After = found
			}
		}
	}
	slices.Sort(locales)
	return outcomes, k.termGate(ctx).fingerprint(ctx, locales), nil
}

// sourceRuns is the source of b, as the change left it.
func sourceRuns(b *model.Block) []model.Run {
	if b == nil {
		return nil
	}
	src, _ := b.Edition(model.EditionKey{})
	return src.Runs
}

// analysisBlock is a block for the analyzers: b's kind and properties, with
// none of the findings a previous check left on it.
func analysisBlock(b *model.Block, source model.LocaleID) *model.Block {
	out := &model.Block{ID: "edition", Translatable: true, SourceLocale: source}
	if b == nil {
		return out
	}
	out.Type, out.MimeType, out.Translatable = b.Type, b.MimeType, b.Translatable
	out.PreserveWhitespace = b.PreserveWhitespace
	if len(b.Properties) > 0 {
		out.Properties = make(map[string]string, len(b.Properties))
		for name, v := range b.Properties {
			switch name {
			case coretools.PropTermCheckErrors, coretools.PropTermCheckWarnings:
				continue
			}
			out.Properties[name] = v
		}
	}
	return out
}

func (k *streamCommitCheck) sourceFindings(ctx context.Context, ch change.EditionChange, runs []model.Run, pc pointChecks) ([]change.Finding, error) {
	b := analysisBlock(ch.Block, k.proj.DefaultSourceLanguage)
	b.SetSourceRuns(runs)
	found, err := voiceFindings(ctx, b, pc)
	if err != nil {
		return nil, err
	}
	return commitFindings(found, ch.Ref), nil
}

func (k *streamCommitCheck) targetFindings(ctx context.Context, ch change.EditionChange, src, runs []model.Run, pc pointChecks) ([]change.Finding, error) {
	b := analysisBlock(ch.Block, k.proj.DefaultSourceLanguage)
	b.SetSourceRuns(src)
	b.SetTargetRuns(ch.Ref.Edition.Locale, runs)
	found, err := standardFindings(ctx, b, pc)
	if err != nil {
		return nil, err
	}
	out := commitFindings(found, ch.Ref)
	if k.termGate(ctx).compliance(ctx, b, ch.Ref.Edition.Locale) == store.TermComplianceViolation {
		at := ch.Ref
		out = append(out, change.Finding{Rule: "terms.vocabulary", Fails: true, At: &at,
			Message: fmt.Sprintf("the %s translation uses a term the terms in force forbid, or leaves out the rendering a source term requires", ch.Ref.Edition.Locale)})
	}
	return out, nil
}

// commitFindings is the check's findings as the change contract reports them,
// at the edition they were found on. A finding a suggested rule raised
// reports and never fails.
func commitFindings(found []check.Finding, at change.Ref) []change.Finding {
	out := make([]change.Finding, 0, len(found))
	for _, f := range found {
		ref := at
		rule := f.Category
		if f.Check != "" {
			rule = f.Check + "." + f.Category
		}
		out = append(out, change.Finding{Rule: rule, Message: f.Message, Fails: f.Fails && !f.Suggested, Suggested: f.Suggested, At: &ref,
			Range: change.FindingRange(&f.Position), Replacement: f.Metadata["replacement"]})
	}
	return out
}
