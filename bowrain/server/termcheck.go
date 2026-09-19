package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/core/voicescope"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/terms"
)

// This file holds the ONE terms-aware term-compliance predicate the ship /
// compliant surfaces and the RV-E review re-check oracle share, so they can never
// disagree on whether a target respects the project's terminology governance.
//
// The predicate combines the two complementary directions the review loop already
// checks (review_recheck.go), lifted here so both surfaces import them:
//
//   - PRESENCE: the target CONTAINS a forbidden or competitor term — drawn from
//     the terms store (LookupAll, the same matcher the concept blast radius and RV-E
//     use) AND from the voice profile's vocabulary (core/profile.MatchVocabulary,
//     the single brand-vocab matcher the voice-vocab-check tool and blast radius
//     call).
//   - ABSENCE: the source uses a concept that MANDATES a preferred or approved
//     rendering for the target locale and the target OMITS it, or the concept is
//     marked do-not-translate and the target does not keep the term verbatim.
//     Both are decided by coretools.TermCheckViolations, the function the
//     term-check tool itself decides with, so a translation the CLI gate passes
//     is compliant here and one it fails is a violation.
//
// The checks are deterministic and offline — no DB or LLM call is made per block.
// The terms store is snapshotted in-memory once per (workspace) and the voice profile
// resolved once per (workspace, locale) by the caller (see termGate), then reused
// across every block of the ship/compliant pass.

// blockTermCompliance is the terminology verdict for a block's committed target
// for tgtLoc. The target is a violation when it USES a forbidden or competitor
// term (presence) or OMITS every rendering its source concept accepts
// (absence, decided as term-check decides it), and compliant when it was
// checked and does neither. It is the single per-block predicate the dashboard ship-state pass,
// the compliant-rate derivation, the review queue, the bulk approve-passing
// endpoint, and the RV-E concept re-check oracle all call.
//
// tb is the terms governing tgtLoc, an in-memory snapshot resolved once by the
// caller (nil when no concept answers for the locale, and the terms half is
// skipped). profile is the voice profile resolved for tgtLoc once by the caller
// (nil when none is bound, and the vocabulary half is skipped). With neither,
// terminology does not govern the locale and the verdict claims nothing. A
// target in a governed locale with no text has nothing to check, so it is
// unchecked. A caller treats both as states of their own, neither compliant
// nor a violation.
func blockTermCompliance(ctx context.Context, block *model.Block, srcLoc, tgtLoc model.LocaleID, tb terms.Terminology, profile *coreprofile.VoiceProfile) store.TermCompliance {
	if tb == nil && coreprofile.BlockRuleCount(profile) == 0 {
		return store.TermComplianceNotGoverned
	}
	if block == nil {
		return store.TermComplianceUnchecked
	}
	targetText := block.TargetText(tgtLoc)
	if strings.TrimSpace(targetText) == "" {
		return store.TermComplianceUnchecked
	}
	// PRESENCE (terms): a forbidden/competitor term appears in the target.
	if tb != nil && targetHasForbiddenTerm(ctx, tb, targetText, tgtLoc) {
		return store.TermComplianceViolation
	}
	// PRESENCE (voice profile): a forbidden/competitor rule or a prohibited style
	// pattern matches the target. core/profile.Findings is the canonical
	// deterministic gate.
	if profile != nil && len(coreprofile.Findings(profile, targetText, nil)) > 0 {
		return store.TermComplianceViolation
	}
	// ABSENCE (terms): the source uses a concept whose mandated rendering for
	// the target locale, or whose do-not-translate term, is missing from the
	// target.
	if tb != nil && targetMissingMandatedTerm(ctx, tb, block.SourceText(), targetText, srcLoc, tgtLoc) {
		return store.TermComplianceViolation
	}
	return store.TermComplianceCompliant
}

// targetHasForbiddenTerm reports whether targetText (a translation in locale loc)
// contains any forbidden or competitor term drawn from tb. It runs the canonical
// terms LookupAll keyed on the target locale so a per-locale terminology
// decision is matched in the language it governs — the same oracle the concept
// blast radius and RV-E's presence direction use.
func targetHasForbiddenTerm(ctx context.Context, tb terms.Terminology, targetText string, loc model.LocaleID) bool {
	matches, err := tb.LookupAll(ctx, targetText, terms.LookupOptions{SourceLocale: loc})
	if err != nil {
		return false
	}
	for _, m := range matches {
		if m.Term.Status == model.TermForbidden || m.Term.CompetitorTerm {
			return true
		}
	}
	return false
}

// targetMissingMandatedTerm reports whether the target VIOLATES terminology by
// ABSENCE: the source uses a concept's term, and the target holds none of the
// renderings that concept accepts for tgtLoc. It decides exactly what the
// term-check tool decides, by deriving the rules with terms.RulesFromConcepts
// and asking coretools.TermCheckViolations, so a translation the CLI gate
// passes is compliant here and one it fails is a violation. That includes an
// admitted or approved term, a declared form of any rendering, an English
// source term's regular inflections, placeholder names read as syntax, and a
// do-not-translate term, which the target must keep verbatim. Redirection
// through USE_INSTEAD / REPLACED_BY relations is not followed, as in term-check.
func targetMissingMandatedTerm(ctx context.Context, tb terms.Terminology, sourceText, targetText string, srcLoc, tgtLoc model.LocaleID) bool {
	if strings.TrimSpace(sourceText) == "" || strings.TrimSpace(targetText) == "" {
		return false
	}
	concepts, err := tb.Concepts(ctx)
	if err != nil || len(concepts) == 0 {
		return false
	}
	rules := terms.RulesFromConcepts(concepts, srcLoc, tgtLoc)
	if len(rules) == 0 {
		return false
	}
	errs, _ := coretools.TermCheckViolations(&coretools.TermCheckConfig{
		TermRules:    rules,
		SourceLocale: srcLoc,
		TargetLocale: tgtLoc,
	}, sourceText, targetText)
	return len(errs) > 0
}

// termGate carries the governance context for one ship/compliant pass: an
// in-memory terms snapshot (resolved once), which target locales that snapshot
// governs (decided at most once per locale), and a per-locale voice profile
// resolver (resolved at most once per locale, then cached). It is the
// caller-side wrapper that keeps the shared blockTermCompliance predicate
// bounded: the pass resolves stores once, the predicate runs offline per block.
// A nil gate holds no governance, so every dimension it is asked about is not
// governed.
type termGate struct {
	srcLoc     model.LocaleID
	tb         terms.Terminology // in-memory snapshot; nil = the workspace holds no terms
	termsFP    string            // digest of the snapshot, for the gate fingerprint
	resolve    func(ctx context.Context, loc model.LocaleID) *coreprofile.VoiceProfile
	profiles   map[model.LocaleID]*coreprofile.VoiceProfile
	profileSet map[model.LocaleID]bool
	// concepts is what tb indexes, read once, and governs caches per target
	// locale whether any of them answers for it.
	concepts     []terms.Concept
	conceptsRead bool
	governs      map[model.LocaleID]bool
}

// newTermGate builds a gate around an already-resolved terms snapshot and a
// per-locale profile resolver. Either input may be absent (nil tb / nil
// resolve). termsFP digests the snapshot so the gate can name the governance a
// stored verdict was computed under.
func newTermGate(srcLoc model.LocaleID, tb terms.Terminology, termsFP string, resolve func(ctx context.Context, loc model.LocaleID) *coreprofile.VoiceProfile) *termGate {
	return &termGate{
		srcLoc:     srcLoc,
		tb:         tb,
		termsFP:    termsFP,
		resolve:    resolve,
		profiles:   map[model.LocaleID]*coreprofile.VoiceProfile{},
		profileSet: map[model.LocaleID]bool{},
		governs:    map[model.LocaleID]bool{},
	}
}

// shipGateAlgorithm names the revision of the per-block ship-gate predicate.
// It rides in every gate fingerprint, so changing what blockFailsChecks or
// blockTermCompliance decide retires every stored verdict rather than leaving
// counters that were computed by code no longer running. Bump it whenever the
// predicate's answer can change for unchanged content.
const shipGateAlgorithm = "4"

// fingerprint names the governance in force for a set of target locales: the
// predicate's own revision, the source language it reads, the terms snapshot,
// and each rated locale's resolved voice profile. Two passes that agree on this
// string judge identical content identically, which is precisely the condition
// under which a stored verdict may be counted instead of recomputed.
//
// Locales are fingerprinted in the order given, so the caller sorts them; the
// alternative is a gate that changes with map iteration order and a corpus that
// is recomputed on every load.
func (g *termGate) fingerprint(ctx context.Context, locales []string) string {
	h := sha256.New()
	fmt.Fprintf(h, "algo=%s\n", shipGateAlgorithm)
	if g != nil {
		fmt.Fprintf(h, "src=%s\nterms=%s\n", g.srcLoc, g.termsFP)
		for _, l := range locales {
			p := g.profileFor(ctx, model.LocaleID(l))
			if p == nil {
				fmt.Fprintf(h, "loc=%s profile=-\n", l)
				continue
			}
			fmt.Fprintf(h, "loc=%s profile=%s v=%d bar=%d at=%d\n",
				l, p.ID, p.Version, p.ComplianceBar(), p.UpdatedAt.UnixNano())
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// profileFor resolves (and caches) the voice profile for one target locale.
// The resolver runs at most once per locale across the whole pass — never per
// block — so the ship/compliant pass adds no per-block store reads.
func (g *termGate) profileFor(ctx context.Context, loc model.LocaleID) *coreprofile.VoiceProfile {
	if g == nil || g.resolve == nil {
		return nil
	}
	if g.profileSet[loc] {
		return g.profiles[loc]
	}
	p := g.resolve(ctx, loc)
	g.profiles[loc] = p
	g.profileSet[loc] = true
	return p
}

// compliance is block's terminology verdict for tgtLoc under the gate's
// governance, from the shared blockTermCompliance predicate. The terms snapshot
// takes part only for a locale it governs, and a nil gate governs nothing.
func (g *termGate) compliance(ctx context.Context, block *model.Block, tgtLoc model.LocaleID) store.TermCompliance {
	if g == nil {
		return store.TermComplianceNotGoverned
	}
	var tb terms.Terminology
	if g.snapshotGoverns(ctx, tgtLoc) {
		tb = g.tb
	}
	return blockTermCompliance(ctx, block, g.srcLoc, tgtLoc, tb, g.profileFor(ctx, tgtLoc))
}

// snapshotGoverns reports whether the terms snapshot governs tgtLoc: whether at
// least one of its concepts answers for a translation from the project's source
// language into it (terms.RuleForConcept), the same derivation kapi's term rule
// resolution applies. A workspace holding terms for other languages only leaves
// tgtLoc ungoverned by terms.
func (g *termGate) snapshotGoverns(ctx context.Context, tgtLoc model.LocaleID) bool {
	if g == nil || g.tb == nil {
		return false
	}
	if governs, ok := g.governs[tgtLoc]; ok {
		return governs
	}
	if !g.conceptsRead {
		concepts, err := g.tb.Concepts(ctx)
		if err != nil {
			// The snapshot is in memory, so a read does not fail in practice.
			// Should one, the gate decides on no concepts and says so.
			slog.ErrorContext(ctx, "termcheck: terms snapshot unreadable; its locales read as ungoverned by terms",
				"error", err)
		}
		g.concepts, g.conceptsRead = concepts, true
	}
	governs := false
	for i := range g.concepts {
		if _, ok := terms.RuleForConcept(g.concepts[i], g.srcLoc, tgtLoc); ok {
			governs = true
			break
		}
	}
	g.governs[tgtLoc] = governs
	return governs
}

// termsGoverned reports whether terminology governs tgtLoc: the terms snapshot
// governs it, or the voice profile resolved for it holds a rule that applies to a
// block. It agrees with compliance, which is not governed for every target of a
// locale this reports false for, and it drives the compliance basis and the
// not-governed count.
func (g *termGate) termsGoverned(ctx context.Context, tgtLoc model.LocaleID) bool {
	if g == nil {
		return false
	}
	return g.snapshotGoverns(ctx, tgtLoc) || coreprofile.BlockRuleCount(g.profileFor(ctx, tgtLoc)) > 0
}

// voiceGoverned reports whether a voice profile applies at tgtLoc, which is what
// puts a voice bar on its blocks.
func (g *termGate) voiceGoverned(ctx context.Context, tgtLoc model.LocaleID) bool {
	return g.profileFor(ctx, tgtLoc) != nil
}

// resolveTermGate builds the terminology-governance gate for a project's
// ship/compliant pass: an in-memory snapshot of the workspace terms (one read,
// reused across every block and locale) plus a per-locale voice profile resolver
// (resolved at most once per locale). The gate is deterministic and offline — no
// per-block DB or LLM call. Returns nil when the project has neither a terms store
// nor a voice store, and a nil gate governs nothing.
func (s *Server) resolveTermGate(ctx context.Context, proj *store.Project, stream, wsID string) *termGate {
	if proj == nil {
		return nil
	}

	// Terms snapshot: read every concept once and index them in memory, so the
	// per-block LookupAll calls never touch the database. An unavailable or empty
	// terms leaves the snapshot nil (the terms store half of the predicate skips).
	var snap terms.Terminology
	var snapFP string
	if tb, err := s.workspaceTermsByID(ctx, wsID); err == nil && tb != nil {
		snap, snapFP = snapshotTerms(ctx, tb)
	}

	// Voice profile resolver: the same hierarchical binding ladder the editor and
	// worker resolve through (voicescope.Resolve), scoped per target locale.
	var resolve func(ctx context.Context, loc model.LocaleID) *coreprofile.VoiceProfile
	if s.VoiceStore != nil {
		var wd voicescope.WorkspaceDefault
		if s.AuthStore != nil {
			wd = &mcpWorkspaceDefaultAdapter{auth: s.AuthStore}
		}
		resolve = func(ctx context.Context, loc model.LocaleID) *coreprofile.VoiceProfile {
			profile, err := voicescope.Resolve(ctx, s.ContentStore, wd, s.VoiceStore, voicescope.Scope{
				WorkspaceID: wsID,
				ProjectID:   proj.ID,
				Stream:      stream,
				Locale:      loc,
			})
			if err != nil {
				return nil
			}
			return profile
		}
	}

	if snap == nil && resolve == nil {
		return nil
	}
	return newTermGate(proj.DefaultSourceLanguage, snap, snapFP, resolve)
}

// snapshotTerms reads every concept from a workspace terms and indexes them
// in a fresh in-memory terms, so the ship/compliant pass can run LookupAll per
// block offline. Returns nil for an empty terms or a read failure (the caller
// then skips the terms store half of the predicate). Relations are not copied: the
// shared predicate does not follow USE_INSTEAD / REPLACED_BY redirection, matching
// the review loop's primitive check.
//
// The second return is a digest of exactly what was indexed — the concepts as
// the snapshot holds them, not as the store spells them — so a gate fingerprint
// changes if and only if the governance the predicate actually applies changed.
// A concept dropped by AddConcept is therefore absent from both the snapshot and
// the digest, which is the honest pairing: verdicts are counted as computed.
func snapshotTerms(ctx context.Context, tb terms.Store) (terms.Terminology, string) {
	concepts, err := tb.Concepts(ctx)
	if err != nil || len(concepts) == 0 {
		return nil, ""
	}
	mem := terms.NewInMemoryStore()
	h := sha256.New()
	for _, c := range concepts {
		// A concept that does not make it into the snapshot is a term the
		// compliant gate then fails to enforce, silently and for that check
		// only. Nothing here can repair it mid-snapshot, so it is logged and
		// the gate runs on what it has.
		if err := mem.AddConcept(ctx, c); err != nil {
			slog.ErrorContext(ctx, "termcheck: concept omitted from the snapshot; it will not be enforced",
				"concept", c.ID, "error", err)
			continue
		}
		// Every field the predicate reads takes part, because the digest's job
		// is to retire a stored verdict exactly when the governance behind it
		// moved. DoNotTranslate decides whether a target must keep the term at
		// all, and a term's declared forms decide which targets satisfy a rule,
		// so a snapshot differing only in those governs differently.
		fmt.Fprintf(h, "%s|%s|%t|", c.ID, c.Domain, c.DoNotTranslate)
		for _, t := range c.Terms {
			fmt.Fprintf(h, "%s/%s/%s/%t/%s,", t.Locale, t.Text, t.Status, t.CompetitorTerm, strings.Join(t.Forms, "+"))
		}
		h.Write([]byte{'\n'})
	}
	return mem, hex.EncodeToString(h.Sum(nil))
}
