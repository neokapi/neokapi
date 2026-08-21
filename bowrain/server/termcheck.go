package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"

	"github.com/neokapi/neokapi/bowrain/core/brandscope"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
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
//     use) AND from the brand profile's vocabulary (core/profile.MatchVocabulary,
//     the single brand-vocab matcher the voice-vocab-check tool and blast radius
//     call).
//   - ABSENCE: the source uses a concept that MANDATES a preferred/approved
//     rendering for the target locale, yet the target OMITS it — the term-check /
//     term-enforce direction (TermEnforceTool's primitives: LookupAll to detect
//     the source term, a case-insensitive contains for the mandated rendering).
//
// The checks are deterministic and offline — no DB or LLM call is made per block.
// The terms store is snapshotted in-memory once per (workspace) and the brand profile
// resolved once per (workspace, locale) by the caller (see termGate), then reused
// across every block of the ship/compliant pass.

// blockTermCompliant reports whether a block's committed target for tgtLoc
// respects the project's terminology governance: it neither USES a
// forbidden/competitor term (presence) nor OMITS a mandated preferred/approved
// rendering its source concept requires (absence). It is the single per-block
// predicate the dashboard ship-state pass, the compliant-rate derivation, the bulk
// approve-passing endpoint, and the RV-E concept re-check oracle all call.
//
// tb is an in-memory terms snapshot resolved once by the caller (nil = the
// project has no terms → the terms store half is skipped). profile is the brand
// voice profile resolved for tgtLoc once by the caller (nil = no brand profile →
// the brand-vocab half is skipped). With both nil the predicate is a pure no-op
// and always reports compliant, so a project with no terminology governance is
// byte-identical to the pre-term behavior. An empty (untranslated) target is
// vacuously compliant — there is nothing to check.
func blockTermCompliant(ctx context.Context, block *model.Block, srcLoc, tgtLoc model.LocaleID, tb terms.Terminology, profile *coreprofile.VoiceProfile) bool {
	if block == nil {
		return true
	}
	targetText := block.TargetText(tgtLoc)
	if strings.TrimSpace(targetText) == "" {
		return true
	}
	// PRESENCE (terms): a forbidden/competitor term appears in the target.
	if tb != nil && targetHasForbiddenTerm(ctx, tb, targetText, tgtLoc) {
		return false
	}
	// PRESENCE (voice profile): a forbidden/competitor rule or a prohibited style
	// pattern matches the target. core/profile.Findings is the canonical
	// deterministic gate.
	if profile != nil && len(coreprofile.Findings(profile, targetText, nil)) > 0 {
		return false
	}
	// ABSENCE (terms): the source uses a concept whose mandated rendering for
	// the target locale is missing from the target.
	if tb != nil && targetMissingMandatedTerm(ctx, tb, block.SourceText(), targetText, srcLoc, tgtLoc) {
		return false
	}
	return true
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
// ABSENCE — the term-check / term-enforce direction, generalized over the whole
// terms. For every concept whose source-locale term appears in sourceText, if
// the concept mandates a preferred/approved rendering for tgtLoc yet none of those
// renderings appears in targetText, the target is non-compliant. It reuses
// TermEnforceTool's primitives (LookupAll to detect the source term, a
// case-insensitive contains for the mandated rendering) without driving the full
// pipeline tool, matching RV-F's already-shipped oracle. Redirection through
// USE_INSTEAD / REPLACED_BY relations is deliberately not followed here — the
// review loop's primitive check does not either, and staying identical to it is
// the point of the shared predicate.
func targetMissingMandatedTerm(ctx context.Context, tb terms.Terminology, sourceText, targetText string, srcLoc, tgtLoc model.LocaleID) bool {
	if strings.TrimSpace(sourceText) == "" || strings.TrimSpace(targetText) == "" {
		return false
	}
	matches, err := tb.LookupAll(ctx, sourceText, terms.LookupOptions{SourceLocale: srcLoc})
	if err != nil || len(matches) == 0 {
		return false
	}
	for _, m := range matches {
		// The renderings this concept mandates for the locale — preferred/approved
		// only, the statuses term-enforce checks. A concept that mandates nothing
		// for the locale has nothing the target can miss.
		var mandated []terms.Term
		for _, tt := range m.Concept.TargetTerms(tgtLoc) {
			if tt.Status == model.TermPreferred || tt.Status == model.TermApproved {
				mandated = append(mandated, tt)
			}
		}
		if len(mandated) == 0 {
			continue
		}
		present := false
		for _, tt := range mandated {
			if terms.ContainsText(targetText, tt.Text, false) {
				present = true
				break
			}
		}
		if !present {
			return true // source uses the concept but the mandated rendering is absent
		}
	}
	return false
}

// termGate carries the terminology-governance context for one ship/compliant pass:
// an in-memory terms snapshot (resolved once) and a per-locale brand-profile
// resolver (resolved at most once per locale, then cached). It is the caller-side
// wrapper that keeps the shared blockTermCompliant predicate bounded — the pass
// resolves stores once, the predicate runs offline per block. A nil gate is a
// no-op (every block compliant, no term governance active), so a project with no
// terms and no brand profile behaves exactly as before.
type termGate struct {
	srcLoc     model.LocaleID
	tb         terms.Terminology // in-memory snapshot; nil = no terms governance
	termsFP    string            // digest of the snapshot, for the gate fingerprint
	resolve    func(ctx context.Context, loc model.LocaleID) *coreprofile.VoiceProfile
	profiles   map[model.LocaleID]*coreprofile.VoiceProfile
	profileSet map[model.LocaleID]bool
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
	}
}

// shipGateAlgorithm names the revision of the per-block ship-gate predicate.
// It rides in every gate fingerprint, so changing what blockFailsChecks or
// blockTermCompliant decide retires every stored verdict rather than leaving
// counters that were computed by code no longer running. Bump it whenever the
// predicate's answer can change for unchanged content.
const shipGateAlgorithm = "1"

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

// profileFor resolves (and caches) the brand voice profile for one target locale.
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

// compliant reports whether block's target for tgtLoc respects terminology
// governance, delegating to the shared blockTermCompliant predicate. A nil gate
// is always compliant.
func (g *termGate) compliant(ctx context.Context, block *model.Block, tgtLoc model.LocaleID) bool {
	if g == nil {
		return true
	}
	return blockTermCompliant(ctx, block, g.srcLoc, tgtLoc, g.tb, g.profileFor(ctx, tgtLoc))
}

// active reports whether the gate has any terminology governance to enforce for
// tgtLoc — a non-empty terms snapshot, or a brand profile carrying forbidden /
// competitor vocabulary. It drives the honest compliance_basis: term checks are
// noted as contributing only where they actually ran against something.
func (g *termGate) active(ctx context.Context, tgtLoc model.LocaleID) bool {
	if g == nil {
		return false
	}
	if g.tb != nil {
		return true
	}
	p := g.profileFor(ctx, tgtLoc)
	return p != nil && (len(p.Vocabulary.ForbiddenTerms) > 0 || len(p.Vocabulary.CompetitorTerms) > 0)
}

// resolveTermGate builds the terminology-governance gate for a project's
// ship/compliant pass: an in-memory snapshot of the workspace terms (one read,
// reused across every block and locale) plus a per-locale brand-profile resolver
// (resolved at most once per locale). The gate is deterministic and offline — no
// per-block DB or LLM call. Returns nil when the project has neither a terms store
// nor a brand store, so the term check is a no-op and the derived ship/compliant
// numbers stay byte-identical to the pre-term behavior.
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

	// Brand-profile resolver: the same hierarchical binding ladder the editor and
	// worker resolve through (brandscope.Resolve), scoped per target locale.
	var resolve func(ctx context.Context, loc model.LocaleID) *coreprofile.VoiceProfile
	if s.VoiceStore != nil {
		var wd brandscope.WorkspaceDefault
		if s.AuthStore != nil {
			wd = &mcpWorkspaceDefaultAdapter{auth: s.AuthStore}
		}
		resolve = func(ctx context.Context, loc model.LocaleID) *coreprofile.VoiceProfile {
			profile, err := brandscope.Resolve(ctx, s.ContentStore, wd, s.VoiceStore, brandscope.Scope{
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
		fmt.Fprintf(h, "%s|%s|", c.ID, c.Domain)
		for _, t := range c.Terms {
			fmt.Fprintf(h, "%s/%s/%s/%t,", t.Locale, t.Text, t.Status, t.CompetitorTerm)
		}
		h.Write([]byte{'\n'})
	}
	return mem, hex.EncodeToString(h.Sum(nil))
}
